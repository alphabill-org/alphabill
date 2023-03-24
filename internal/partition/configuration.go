package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/txbuffer"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

const (
	DefaultT1Timeout                   = 750 * time.Millisecond
	DefaultTxBufferSize                = 1000
	DefaultReplicationMaxBlocks uint64 = 1000
	DefaultReplicationMaxTx     uint32 = 10 * DefaultTxBufferSize
)

var (
	ErrTxSystemIsNil       = errors.New("transaction system is nil")
	ErrPeerIsNil           = errors.New("peer is nil")
	ErrGenesisIsNil        = errors.New("genesis is nil")
	ErrInvalidRootHash     = errors.New("tx system root hash does not equal to genesis file hash")
	ErrInvalidSummaryValue = errors.New("tx system summary value does not equal to genesis file summary value")
)

type (
	configuration struct {
		context                     context.Context
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		leaderSelector              LeaderSelector
		blockStore                  keyvaluedb.KeyValueDB
		txBuffer                    *txbuffer.TxBuffer
		t1Timeout                   time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.
		hashAlgorithm               gocrypto.Hash // make hash algorithm configurable in the future. currently it is using SHA-256.
		peer                        *network.Peer
		signer                      crypto.Signer
		genesis                     *genesis.PartitionGenesis
		rootTrustBase               map[string]crypto.Verifier
		rootChainAddress            multiaddr.Multiaddr
		rootChainID                 peer.ID
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

func WithContext(context context.Context) NodeOption {
	return func(c *configuration) {
		c.context = context
	}
}

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

func WithT1Timeout(t1Timeout time.Duration) NodeOption {
	return func(c *configuration) {
		c.t1Timeout = t1Timeout
	}
}

func WithRootAddressAndIdentifier(address multiaddr.Multiaddr, id peer.ID) NodeOption {
	return func(c *configuration) {
		c.rootChainAddress = address
		c.rootChainID = id
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

func loadAndValidateConfiguration(peer *network.Peer, signer crypto.Signer, genesis *genesis.PartitionGenesis, txs txsystem.TransactionSystem, net Net, nodeOptions ...NodeOption) (*configuration, error) {
	if peer == nil {
		return nil, ErrPeerIsNil
	}
	if signer == nil {
		return nil, ErrSignerIsNil
	}
	if genesis == nil {
		return nil, ErrGenesisIsNil
	}
	if txs == nil {
		return nil, ErrTxSystemIsNil
	}
	if net == nil {
		return nil, errors.New("network is nil")
	}
	c := &configuration{
		peer:          peer,
		signer:        signer,
		genesis:       genesis,
		hashAlgorithm: gocrypto.SHA256,
	}
	for _, option := range nodeOptions {
		option(c)
	}
	// init default for those not specified by the user
	if err := c.initMissingDefaults(peer); err != nil {
		return nil, fmt.Errorf("failed to initiate default parameters, %w", err)
	}
	if err := c.isGenesisValid(txs); err != nil {
		return nil, fmt.Errorf("genesis error, %w", err)
	}
	return c, nil
}

// initMissingDefaults loads missing default configuration.
func (c *configuration) initMissingDefaults(peer *network.Peer) error {
	if c.context == nil {
		c.context = context.Background()
	}
	if c.t1Timeout == 0 {
		c.t1Timeout = DefaultT1Timeout
	}
	if c.blockStore == nil {
		c.blockStore = memorydb.New()
	}

	var err error
	c.txBuffer, err = txbuffer.New(DefaultTxBufferSize, c.hashAlgorithm)
	if err != nil {
		return fmt.Errorf("tx buffer init error, %w", err)
	}

	if c.leaderSelector == nil {
		c.leaderSelector, err = NewDefaultLeaderSelector(peer, c.GetSystemIdentifier())
		if err != nil {
			return fmt.Errorf("leader election init error, %w", err)
		}
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
	if c.rootChainAddress != nil {
		// add rootchain address to the peer store. this enables us to send receivedMessages to the rootchain.
		c.peer.Network().Peerstore().AddAddr(c.rootChainID, c.rootChainAddress, peerstore.PermanentAddrTTL)
	}
	if c.replicationConfig.maxBlocks == 0 {
		c.replicationConfig.maxBlocks = DefaultReplicationMaxBlocks
	}
	if c.replicationConfig.maxTx == 0 {
		c.replicationConfig.maxTx = DefaultReplicationMaxTx
	}
	return nil
}

func (c *configuration) genesisBlock() *block.Block {
	return &block.Block{
		SystemIdentifier:   c.genesis.SystemDescriptionRecord.SystemIdentifier,
		NodeIdentifier:     "genesis",
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: c.genesis.GetCertificate(),
	}
}

func (c *configuration) isGenesisValid(txs txsystem.TransactionSystem) error {
	if err := c.genesis.IsValid(c.rootTrustBase, c.hashAlgorithm); err != nil {
		logger.Warning("Invalid partition genesis file: %v", err)
		return fmt.Errorf("invalid partition genesis file, %w", err)
	}
	state, err := txs.State()
	if err != nil {
		return err
	}
	txGenesisRoot := state.Root()
	txSummaryValue := state.Summary()
	genesisCertificate := c.genesis.Certificate
	genesisInputRecord := genesisCertificate.InputRecord
	if !bytes.Equal(genesisInputRecord.Hash, txGenesisRoot) {
		logger.Warning("tx system root hash does not equal to genesis file hash. "+
			"genesis hash: %X, txSystem hash: %X", genesisInputRecord.Hash, txGenesisRoot)
		return ErrInvalidRootHash
	}

	if !bytes.Equal(genesisInputRecord.SummaryValue, txSummaryValue) {
		logger.Warning("tx system summary value does not equal to genesis file summary value. "+
			"Genesis SummaryValue: %X, TxSystem SummaryValue: %X", genesisInputRecord.SummaryValue, txSummaryValue)
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
