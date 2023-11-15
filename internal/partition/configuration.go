package partition

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	DefaultT1Timeout                   = 750 * time.Millisecond
	DefaultTxBufferSize                = 1000
	DefaultReplicationMaxBlocks uint64 = 1000
	DefaultReplicationMaxTx     uint32 = 10 * DefaultTxBufferSize
)

var (
	ErrTxSystemIsNil       = errors.New("transaction system is nil")
	ErrGenesisIsNil        = errors.New("genesis is nil")
	ErrInvalidRootHash     = errors.New("tx system root hash does not equal to genesis file hash")
	ErrInvalidSummaryValue = errors.New("tx system summary value does not equal to genesis file summary value")
)

type (
	configuration struct {
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		leaderSelector              LeaderSelector
		blockStore                  keyvaluedb.KeyValueDB
		txIndexer                   keyvaluedb.KeyValueDB
		t1Timeout                   time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.
		hashAlgorithm               gocrypto.Hash // make hash algorithm configurable in the future. currently it is using SHA-256.
		signer                      crypto.Signer
		genesis                     *genesis.PartitionGenesis
		rootTrustBase               map[string]crypto.Verifier
		eventHandler                event.Handler
		eventChCapacity             int
		replicationConfig           ledgerReplicationConfig
	}

	NodeOption func(c *configuration)

	ledgerReplicationConfig struct {
		maxBlocks uint64
		maxTx     uint32
	}
)

func WithReplicationParams(maxBlocks uint64, maxTx uint32) NodeOption {
	return func(c *configuration) {
		c.replicationConfig.maxBlocks = maxBlocks
		c.replicationConfig.maxTx = maxTx
	}
}

func WithUnicityCertificateValidator(unicityCertificateValidator UnicityCertificateValidator) NodeOption {
	return func(c *configuration) {
		c.unicityCertificateValidator = unicityCertificateValidator
	}
}

func WithBlockProposalValidator(blockProposalValidator BlockProposalValidator) NodeOption {
	return func(c *configuration) {
		c.blockProposalValidator = blockProposalValidator
	}
}

func WithLeaderSelector(leaderSelector LeaderSelector) NodeOption {
	return func(c *configuration) {
		c.leaderSelector = leaderSelector
	}
}

func WithBlockStore(blockStore keyvaluedb.KeyValueDB) NodeOption {
	return func(c *configuration) {
		c.blockStore = blockStore
	}
}

func WithTxIndexer(txIndexer keyvaluedb.KeyValueDB) NodeOption {
	return func(c *configuration) {
		c.txIndexer = txIndexer
	}
}

func WithT1Timeout(t1Timeout time.Duration) NodeOption {
	return func(c *configuration) {
		c.t1Timeout = t1Timeout
	}
}

func WithEventHandler(eh event.Handler, eventChCapacity int) NodeOption {
	return func(c *configuration) {
		c.eventHandler = eh
		c.eventChCapacity = eventChCapacity
	}
}

func WithTxValidator(txValidator TxValidator) NodeOption {
	return func(c *configuration) {
		c.txValidator = txValidator
	}
}

func loadAndValidateConfiguration(signer crypto.Signer, genesis *genesis.PartitionGenesis, txs txsystem.TransactionSystem, log *slog.Logger, nodeOptions ...NodeOption) (*configuration, error) {
	if signer == nil {
		return nil, ErrSignerIsNil
	}
	if genesis == nil {
		return nil, ErrGenesisIsNil
	}
	if txs == nil {
		return nil, ErrTxSystemIsNil
	}
	c := &configuration{
		signer:        signer,
		genesis:       genesis,
		hashAlgorithm: gocrypto.SHA256,
	}
	for _, option := range nodeOptions {
		option(c)
	}
	// init default for those not specified by the user
	if err := c.initMissingDefaults(); err != nil {
		return nil, fmt.Errorf("failed to initiate default parameters, %w", err)
	}
	if err := c.isGenesisValid(txs); err != nil {
		return nil, fmt.Errorf("genesis error, %w", err)
	}
	return c, nil
}

// initMissingDefaults loads missing default configuration.
func (c *configuration) initMissingDefaults() error {
	if c.t1Timeout == 0 {
		c.t1Timeout = DefaultT1Timeout
	}
	if c.blockStore == nil {
		c.blockStore = memorydb.New()
	}

	var err error

	if c.leaderSelector == nil {
		c.leaderSelector = NewDefaultLeaderSelector()
	}
	c.rootTrustBase, err = genesis.NewValidatorTrustBase(c.genesis.RootValidators)
	if err != nil {
		return fmt.Errorf("root trust base init error, %w", err)
	}
	if c.blockProposalValidator == nil {

		c.blockProposalValidator, err = NewDefaultBlockProposalValidator(c.genesis.SystemDescriptionRecord, c.rootTrustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("block proposal validator init error, %w", err)
		}
	}
	if c.unicityCertificateValidator == nil {
		c.unicityCertificateValidator, err = NewDefaultUnicityCertificateValidator(c.genesis.SystemDescriptionRecord, c.rootTrustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("unicity certificate validator init error, %w", err)
		}
	}
	if c.txValidator == nil {
		c.txValidator, err = NewDefaultTxValidator(c.GetSystemIdentifier())
		if err != nil {
			return err
		}
	}
	if c.replicationConfig.maxBlocks == 0 {
		c.replicationConfig.maxBlocks = DefaultReplicationMaxBlocks
	}
	if c.replicationConfig.maxTx == 0 {
		c.replicationConfig.maxTx = DefaultReplicationMaxTx
	}
	return nil
}

func (c *configuration) genesisBlock() *types.Block {
	return &types.Block{
		Header: &types.Header{
			SystemID:   c.genesis.SystemDescriptionRecord.SystemIdentifier,
			ProposerID: "genesis",
		},
		Transactions:       []*types.TransactionRecord{},
		UnicityCertificate: c.genesis.Certificate,
	}
}

func (c *configuration) isGenesisValid(txs txsystem.TransactionSystem) error {
	if err := c.genesis.IsValid(c.rootTrustBase, c.hashAlgorithm); err != nil {
		return fmt.Errorf("invalid partition genesis file: %w", err)
	}
	state, err := txs.StateSummary()
	if err != nil {
		return err
	}
	txGenesisRoot := state.Root()
	txSummaryValue := state.Summary()
	genesisCertificate := c.genesis.Certificate
	genesisInputRecord := genesisCertificate.InputRecord
	if !bytes.Equal(genesisInputRecord.Hash, txGenesisRoot) {
		return ErrInvalidRootHash
	}

	if !bytes.Equal(genesisInputRecord.SummaryValue, txSummaryValue) {
		return ErrInvalidSummaryValue
	}
	return nil
}

func (c *configuration) GetSystemIdentifier() []byte {
	return c.genesis.SystemDescriptionRecord.SystemIdentifier
}

func (c *configuration) GetT2Timeout() time.Duration {
	return time.Duration(c.genesis.SystemDescriptionRecord.T2Timeout) * time.Millisecond
}

func (c *configuration) GetSigningPublicKey(nodeIdentifier string) (crypto.Verifier, error) {
	for _, key := range c.genesis.Keys {
		if key.NodeIdentifier == nodeIdentifier {
			return crypto.NewVerifierSecp256k1(key.SigningPublicKey)
		}
	}
	return nil, fmt.Errorf("signing public key for id %v not found", nodeIdentifier)
}

func (c *configuration) getRootNodes() (peer.IDSlice, error) {
	nodes := make(peer.IDSlice, len(c.genesis.RootValidators))
	for i, node := range c.genesis.RootValidators {
		id, err := node.NodeID()
		if err != nil {
			return nil, fmt.Errorf("invalid root node id error: %w", err)
		}
		nodes[i] = id
	}
	sort.Sort(nodes)
	return nodes, nil
}
