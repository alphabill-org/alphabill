package partition

import (
	"crypto"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/txsystem"
)

const (
	DefaultT1Timeout                       = 750 * time.Millisecond
	DefaultReplicationMaxBlocks     uint64 = 1000
	DefaultReplicationMaxTx         uint32 = 10000
	DefaultBlockSubscriptionTimeout        = 3000 * time.Millisecond
	DefaultLedgerReplicationTimeout        = 1500 * time.Millisecond
)

var (
	ErrTxSystemIsNil  = errors.New("transaction system is nil")
	ErrGenesisIsNil   = errors.New("genesis is nil")
	ErrTrustBaseIsNil = errors.New("trust base is nil")
)

type (
	configuration struct {
		shardID                     types.ShardID
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  keyvaluedb.KeyValueDB
		shardStore                  keyvaluedb.KeyValueDB
		proofIndexConfig            proofIndexConfig
		ownerIndexer                *OwnerIndexer
		t1Timeout                   time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.
		hashAlgorithm               crypto.Hash   // make hash algorithm configurable in the future. currently it is using SHA-256.
		signer                      abcrypto.Signer
		genesis                     *genesis.PartitionGenesis
		trustBase                   types.RootTrustBase
		eventHandler                event.Handler
		eventChCapacity             int
		replicationConfig           ledgerReplicationConfig
		blockSubscriptionTimeout    time.Duration // time since last block when to start recovery on non-validating node
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
		maxFetchBlocks  uint64
		maxReturnBlocks uint64
		maxTx           uint32
		timeout         time.Duration
	}
)

func WithReplicationParams(maxFetchBlocks, maxReturnBlocks uint64, maxTx uint32, timeout time.Duration) NodeOption {
	return func(c *configuration) {
		c.replicationConfig.maxFetchBlocks = maxFetchBlocks
		c.replicationConfig.maxReturnBlocks = maxReturnBlocks
		c.replicationConfig.maxTx = maxTx
		c.replicationConfig.timeout = timeout
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

func WithBlockStore(blockStore keyvaluedb.KeyValueDB) NodeOption {
	return func(c *configuration) {
		c.blockStore = blockStore
	}
}

func WithShardStore(shardStore keyvaluedb.KeyValueDB) NodeOption {
	return func(c *configuration) {
		c.shardStore = shardStore
	}
}

func WithProofIndex(db keyvaluedb.KeyValueDB, history uint64) NodeOption {
	return func(c *configuration) {
		c.proofIndexConfig.store = db
		c.proofIndexConfig.historyLen = history
	}
}

func WithOwnerIndex(ownerIndexer *OwnerIndexer) NodeOption {
	return func(c *configuration) {
		c.ownerIndexer = ownerIndexer
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

func WithBlockSubscriptionTimeout(t time.Duration) NodeOption {
	return func(c *configuration) {
		c.blockSubscriptionTimeout = t
	}
}

func loadAndValidateConfiguration(signer abcrypto.Signer, genesis *genesis.PartitionGenesis, trustBase types.RootTrustBase, txs txsystem.TransactionSystem, nodeOptions ...NodeOption) (*configuration, error) {
	if signer == nil {
		return nil, ErrSignerIsNil
	}
	if genesis == nil {
		return nil, ErrGenesisIsNil
	}
	if trustBase == nil {
		return nil, ErrTrustBaseIsNil
	}
	if txs == nil {
		return nil, ErrTxSystemIsNil
	}
	c := &configuration{
		signer:        signer,
		genesis:       genesis,
		hashAlgorithm: crypto.SHA256,
		proofIndexConfig: proofIndexConfig{
			historyLen: 20,
		},
		trustBase: trustBase,
	}
	for _, option := range nodeOptions {
		option(c)
	}
	// init default for those not specified by the user
	if err := c.initMissingDefaults(); err != nil {
		return nil, fmt.Errorf("initializing missing configuration to default values: %w", err)
	}
	if err := c.genesis.IsValid(c.trustBase, c.hashAlgorithm); err != nil {
		return nil, fmt.Errorf("invalid genesis: %w", err)
	}

	return c, nil
}

// initMissingDefaults loads missing default configuration.
func (c *configuration) initMissingDefaults() error {
	if c.t1Timeout == 0 {
		c.t1Timeout = DefaultT1Timeout
	}

	var err error
	if c.proofIndexConfig.store == nil {
		if c.proofIndexConfig.store, err = memorydb.New(); err != nil {
			return fmt.Errorf("creating proof index DB: %w", err)
		}
	}

	if c.blockStore == nil {
		if c.blockStore, err = memorydb.New(); err != nil {
			return fmt.Errorf("creating block store DB: %w", err)
		}
	}

	if c.shardStore == nil {
		if c.shardStore, err = memorydb.New(); err != nil {
			return fmt.Errorf("creating shard store DB: %w", err)
		}
	}

	if c.blockProposalValidator == nil {
		c.blockProposalValidator, err = NewDefaultBlockProposalValidator(c.genesis.PartitionDescription, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("initializing block proposal validator: %w", err)
		}
	}
	if c.unicityCertificateValidator == nil {
		c.unicityCertificateValidator, err = NewDefaultUnicityCertificateValidator(c.genesis.PartitionDescription, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("initializing unicity certificate validator: %w", err)
		}
	}
	if c.txValidator == nil {
		c.txValidator, err = NewDefaultTxValidator(c.GetPartitionID())
		if err != nil {
			return err
		}
	}
	if c.replicationConfig.maxFetchBlocks == 0 {
		c.replicationConfig.maxFetchBlocks = DefaultReplicationMaxBlocks
	}
	if c.replicationConfig.maxReturnBlocks == 0 {
		c.replicationConfig.maxReturnBlocks = DefaultReplicationMaxBlocks
	}
	if c.replicationConfig.maxTx == 0 {
		c.replicationConfig.maxTx = DefaultReplicationMaxTx
	}
	if c.replicationConfig.timeout == 0 {
		c.replicationConfig.timeout = DefaultLedgerReplicationTimeout
	}
	if c.blockSubscriptionTimeout == 0 {
		c.blockSubscriptionTimeout = DefaultBlockSubscriptionTimeout
	}
	return nil
}

func (c *configuration) GetNetworkID() types.NetworkID {
	return c.genesis.PartitionDescription.NetworkID
}

func (c *configuration) GetPartitionID() types.PartitionID {
	return c.genesis.PartitionDescription.PartitionID
}

func (c *configuration) GetT2Timeout() time.Duration {
	return c.genesis.PartitionDescription.T2Timeout
}

func (c *configuration) getRootNodes() (peer.IDSlice, error) {
	nodes := c.trustBase.GetRootNodes()
	idSlice := make(peer.IDSlice, len(nodes))
	for i, node := range nodes {
		id, err := peer.Decode(node.NodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid root node id in trust base: %w", err)
		}
		idSlice[i] = id
	}
	return idSlice, nil
}
