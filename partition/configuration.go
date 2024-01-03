package partition

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	DefaultT1Timeout                   = 750 * time.Millisecond
	DefaultReplicationMaxBlocks uint64 = 1000
	DefaultReplicationMaxTx     uint32 = 10000
)

var (
	ErrTxSystemIsNil = errors.New("transaction system is nil")
	ErrGenesisIsNil  = errors.New("genesis is nil")
)

type (
	configuration struct {
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		leaderSelector              LeaderSelector
		blockStore                  keyvaluedb.KeyValueDB
		proofIndexConfig            proofIndexConfig
		t1Timeout                   time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.
		hashAlgorithm               gocrypto.Hash // make hash algorithm configurable in the future. currently it is using SHA-256.
		signer                      abcrypto.Signer
		genesis                     *genesis.PartitionGenesis
		rootTrustBase               map[string]abcrypto.Verifier
		eventHandler                event.Handler
		eventChCapacity             int
		replicationConfig           ledgerReplicationConfig
	}

	NodeOption func(c *configuration)
	// proofIndexConfig proof indexer config
	// store - type of store, either a memory DB or bolt DB
	// historyLen - number of rounds/blocks to keep in indexer:
	// - if 0, there is no clean-up and all blocks are kept in the index;
	// - otherwise, the latest historyLen is kept and older will be removed from the DB (sliding window).
	proofIndexConfig struct {
		store      keyvaluedb.KeyValueDB
		historyLen uint64
	}

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

func WithProofIndex(db keyvaluedb.KeyValueDB, history uint64) NodeOption {
	return func(c *configuration) {
		c.proofIndexConfig.store = db
		c.proofIndexConfig.historyLen = history
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

func loadAndValidateConfiguration(signer abcrypto.Signer, genesis *genesis.PartitionGenesis, txs txsystem.TransactionSystem, log *slog.Logger, nodeOptions ...NodeOption) (*configuration, error) {
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
		proofIndexConfig: proofIndexConfig{
			store:      memorydb.New(),
			historyLen: 20,
		},
	}
	for _, option := range nodeOptions {
		option(c)
	}
	// init default for those not specified by the user
	if err := c.initMissingDefaults(); err != nil {
		return nil, fmt.Errorf("failed to initiate default parameters, %w", err)
	}
	if err := c.genesis.IsValid(c.rootTrustBase, c.hashAlgorithm); err != nil {
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
	if c.leaderSelector == nil {
		c.leaderSelector = NewDefaultLeaderSelector()
	}

	var err error
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

func (c *configuration) GetSystemIdentifier() types.SystemID {
	return c.genesis.SystemDescriptionRecord.SystemIdentifier
}

func (c *configuration) GetT2Timeout() time.Duration {
	return time.Duration(c.genesis.SystemDescriptionRecord.T2Timeout) * time.Millisecond
}

func (c *configuration) GetSigningPublicKey(nodeIdentifier string) (abcrypto.Verifier, error) {
	for _, key := range c.genesis.Keys {
		if key.NodeIdentifier == nodeIdentifier {
			return abcrypto.NewVerifierSecp256k1(key.SigningPublicKey)
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
