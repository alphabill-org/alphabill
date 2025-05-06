package partition

import (
	"crypto"
	"errors"
	"fmt"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/partition/event"
)

const (
	KeyAlgorithmSecp256k1 = "secp256k1"

	DefaultT1Timeout                       = 750
	DefaultReplicationMaxBlocks     uint64 = 1000
	DefaultReplicationMaxTx         uint32 = 10000
	DefaultBlockSubscriptionTimeout        = 3000 * time.Millisecond
	DefaultLedgerReplicationTimeout        = 1500 * time.Millisecond
)

var (
	ErrShardConfIsNil = errors.New("shard configuration is nil")
	ErrKeyConfIsNil   = errors.New("key configuration is nil")
	ErrTrustBaseIsNil = errors.New("trust base is nil")
)

type (
	NodeConf struct {
		keyConf       *KeyConf
		shardConf     *types.PartitionDescriptionRecord
		trustBase     types.RootTrustBase
		observability Observability

		address            string
		announceAddresses  []string
		bootstrapAddresses []string
		validatorNetwork   ValidatorNetwork

		signer           abcrypto.Signer
		hashAlgorithm    crypto.Hash // make hash algorithm configurable in the future. currently it is using SHA-256.
		txValidator      TxValidator
		ucValidator      UnicityCertificateValidator
		bpValidator      BlockProposalValidator
		blockStore       keyvaluedb.KeyValueDB
		shardStore       keyvaluedb.KeyValueDB
		proofIndexConfig proofIndexConfig
		ownerIndexer     *OwnerIndexer
		t1Timeout        time.Duration // T1 timeout of the node. Time to wait before node creates a new block proposal.

		eventHandler             event.Handler
		eventChCapacity          int
		replicationConfig        ledgerReplicationConfig
		blockSubscriptionTimeout time.Duration // time since last block when to start recovery on non-validating node
	}

	NodeOption func(c *NodeConf)

	KeyConf struct {
		SigKey  Key `json:"sigKey"`
		AuthKey Key `json:"authKey"`
	}

	Key struct {
		Algorithm  string    `json:"algorithm"`
		PrivateKey hex.Bytes `json:"privateKey"`
	}

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

func NewNodeConf(
	keyConf *KeyConf,
	shardConf *types.PartitionDescriptionRecord,
	trustBase types.RootTrustBase,
	observability Observability,
	nodeOptions ...NodeOption,
) (*NodeConf, error) {
	if keyConf == nil {
		return nil, ErrKeyConfIsNil
	}
	if shardConf == nil {
		return nil, ErrShardConfIsNil
	}
	if trustBase == nil {
		return nil, ErrTrustBaseIsNil
	}
	signer, err := keyConf.Signer()
	if err != nil {
		return nil, err
	}

	c := &NodeConf{
		keyConf:       keyConf,
		shardConf:     shardConf,
		trustBase:     trustBase,
		signer:        signer,
		hashAlgorithm: crypto.SHA256,
		proofIndexConfig: proofIndexConfig{
			historyLen: 20,
		},
		observability: observability,
	}
	for _, option := range nodeOptions {
		option(c)
	}
	// init default for those not specified by the user
	if err := c.initMissingDefaults(); err != nil {
		return nil, fmt.Errorf("initializing missing configuration to default values: %w", err)
	}
	if err := c.shardConf.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid shard configuration: %w", err)
	}

	return c, nil
}

func WithAddress(address string) NodeOption {
	return func(c *NodeConf) {
		c.address = address
	}
}

func WithAnnounceAddresses(announceAddresses []string) NodeOption {
	return func(c *NodeConf) {
		c.announceAddresses = announceAddresses
	}
}

func WithBootstrapAddresses(bootstrapAddresses []string) NodeOption {
	return func(c *NodeConf) {
		c.bootstrapAddresses = bootstrapAddresses
	}
}

func WithValidatorNetwork(validatorNetwork ValidatorNetwork) NodeOption {
	return func(c *NodeConf) {
		c.validatorNetwork = validatorNetwork
	}
}

func WithReplicationParams(maxFetchBlocks, maxReturnBlocks uint64, maxTx uint32, timeout time.Duration) NodeOption {
	return func(c *NodeConf) {
		c.replicationConfig.maxFetchBlocks = maxFetchBlocks
		c.replicationConfig.maxReturnBlocks = maxReturnBlocks
		c.replicationConfig.maxTx = maxTx
		c.replicationConfig.timeout = timeout
	}
}

func WithUnicityCertificateValidator(unicityCertificateValidator UnicityCertificateValidator) NodeOption {
	return func(c *NodeConf) {
		c.ucValidator = unicityCertificateValidator
	}
}

func WithBlockProposalValidator(blockProposalValidator BlockProposalValidator) NodeOption {
	return func(c *NodeConf) {
		c.bpValidator = blockProposalValidator
	}
}

func WithBlockStore(blockStore keyvaluedb.KeyValueDB) NodeOption {
	return func(c *NodeConf) {
		c.blockStore = blockStore
	}
}

func WithShardStore(shardStore keyvaluedb.KeyValueDB) NodeOption {
	return func(c *NodeConf) {
		c.shardStore = shardStore
	}
}

func WithProofIndex(db keyvaluedb.KeyValueDB, history uint64) NodeOption {
	return func(c *NodeConf) {
		c.proofIndexConfig.store = db
		c.proofIndexConfig.historyLen = history
	}
}

func WithOwnerIndex(ownerIndexer *OwnerIndexer) NodeOption {
	return func(c *NodeConf) {
		c.ownerIndexer = ownerIndexer
	}
}

func WithT1Timeout(t1Timeout time.Duration) NodeOption {
	return func(c *NodeConf) {
		c.t1Timeout = t1Timeout
	}
}

func WithEventHandler(eh event.Handler, eventChCapacity int) NodeOption {
	return func(c *NodeConf) {
		c.eventHandler = eh
		c.eventChCapacity = eventChCapacity
	}
}

func WithTxValidator(txValidator TxValidator) NodeOption {
	return func(c *NodeConf) {
		c.txValidator = txValidator
	}
}

func WithBlockSubscriptionTimeout(t time.Duration) NodeOption {
	return func(c *NodeConf) {
		c.blockSubscriptionTimeout = t
	}
}

// initMissingDefaults loads missing default configuration.
func (c *NodeConf) initMissingDefaults() error {
	if c.t1Timeout == 0 {
		c.t1Timeout = DefaultT1Timeout * time.Millisecond
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

	if c.bpValidator == nil {
		c.bpValidator, err = NewDefaultBlockProposalValidator(
			c.shardConf.PartitionID, c.shardConf.ShardID, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("initializing block proposal validator: %w", err)
		}
	}
	if c.ucValidator == nil {
		c.ucValidator, err = NewDefaultUnicityCertificateValidator(
			c.shardConf.PartitionID, c.shardConf.ShardID, c.trustBase, c.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("initializing unicity certificate validator: %w", err)
		}
	}
	if c.txValidator == nil {
		c.txValidator, err = NewDefaultTxValidator(c.PartitionID())
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

func (c *NodeConf) NetworkID() types.NetworkID {
	return c.shardConf.NetworkID
}

func (c *NodeConf) PartitionID() types.PartitionID {
	return c.shardConf.PartitionID
}

func (c *NodeConf) ShardID() types.ShardID {
	return c.shardConf.ShardID
}

func (c *NodeConf) GetT2Timeout() time.Duration {
	return c.shardConf.T2Timeout
}

func (c *NodeConf) ShardConf() *types.PartitionDescriptionRecord {
	return c.shardConf
}

func (c *NodeConf) Orchestration() Orchestration {
	return Orchestration{c.trustBase}
}

func (c *NodeConf) Observability() Observability {
	return c.observability
}

func (c *NodeConf) HashAlgorithm() crypto.Hash {
	return c.hashAlgorithm
}

func (c *NodeConf) BlockStore() keyvaluedb.KeyValueDB {
	return c.blockStore
}

func (c *NodeConf) ProofStore() keyvaluedb.KeyValueDB {
	return c.proofIndexConfig.store
}

func (c *NodeConf) PeerConf() (*network.PeerConfiguration, error) {
	authKeyPair, err := c.keyConf.AuthKeyPair()
	if err != nil {
		return nil, fmt.Errorf("invalid authentication key: %w", err)
	}

	bootNodes := make([]peer.AddrInfo, len(c.bootstrapAddresses))
	for i, addr := range c.bootstrapAddresses {
		addrInfo, err := peer.AddrInfoFromString(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap address: %w", err)
		}
		bootNodes[i] = *addrInfo
	}

	return network.NewPeerConfiguration(c.address, c.announceAddresses, authKeyPair, bootNodes)
}

func (c *NodeConf) OwnerIndexer() *OwnerIndexer {
	return c.ownerIndexer
}

func (c *NodeConf) getRootNodes() (peer.IDSlice, error) {
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

func (c *KeyConf) NodeID() (peer.ID, error) {
	authPrivKey, err := p2pcrypto.UnmarshalSecp256k1PrivateKey(c.AuthKey.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid authentication key: %w", err)
	}
	return peer.IDFromPrivateKey(authPrivKey)
}

func (c *KeyConf) Signer() (abcrypto.Signer, error) {
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(c.SigKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid signing key: %w", err)
	}
	return signer, nil
}

func (c *KeyConf) AuthKeyPair() (*network.PeerKeyPair, error) {
	if c.AuthKey.Algorithm != KeyAlgorithmSecp256k1 {
		return nil, fmt.Errorf("unsupported authentication key algorithm %v", c.AuthKey.Algorithm)
	}
	authPrivKey, err := p2pcrypto.UnmarshalSecp256k1PrivateKey(c.AuthKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal authentication key: %w", err)
	}
	authPrivKeyBytes, err := authPrivKey.Raw()
	if err != nil {
		return nil, err
	}
	authPubKeyBytes, err := authPrivKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	return &network.PeerKeyPair{
		PublicKey:  authPubKeyBytes,
		PrivateKey: authPrivKeyBytes,
	}, nil
}
