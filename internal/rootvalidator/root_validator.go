package rootvalidator

import (
	"context"
	gocrypto "crypto"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootchain"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/store"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	defaultRoundTimeout = 1000 * time.Millisecond
	defaultLocalTimeout = 10000 * time.Millisecond
	// local timeout
	blockRateId    = "block-rate"
	localTimeoutId = "local-timeout"
)

type (
	RootNodeConf struct {
		BlockRateMs        time.Duration
		LocalTimeoutMs     time.Duration
		ConsensusThreshold uint32
		RootTrustBase      map[string]crypto.Verifier
		HashAlgorithm      gocrypto.Hash
		stateStore         StateStore
	}
	Option func(c *RootNodeConf)

	Validator struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		conf             *RootNodeConf
		partitionHost    *network.Peer // p2p network host for partition
		rootHost         *network.Peer // p2p network host for root validators
		partitionStore   *rootchain.PartitionStore
		partitionManager *PartitionManager
		consensusManager *AtomicBroadcastManager // Handles root validator communication and consensus.
	}
)

func WithStateStore(store StateStore) Option {
	return func(c *RootNodeConf) {
		c.stateStore = store
	}
}

// NewRootValidatorNode creates a new instance of the root validator node
func NewRootValidatorNode(
	prt *network.Peer,
	rootHost *network.Peer,
	g *genesis.RootGenesis,
	signer crypto.Signer,
	pNet PartitionNet,
	rNet RootNet,
	opts ...Option,
) (*Validator, error) {
	if prt == nil {
		return nil, errors.New("partition listener is nil")
	}
	if rootHost == nil {
		return nil, errors.New("Root host is nil")
	}
	log.SetContext(log.KeyNodeID, prt.ID().String())
	if pNet == nil {
		return nil, errors.New("network is nil")
	}
	if rNet == nil {
		return nil, errors.New("network is nil")
	}
	if g == nil {
		return nil, errors.New("root chain genesis is nil")
	}
	err := g.Verify()
	if err != nil {
		return nil, errors.New("invalid root genesis file")
	}
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", prt.ID(), prt.MultiAddresses())
	configuration := loadConf(g.Root, opts)
	// Load/initiate state store
	stateStore := configuration.stateStore
	state, err := stateStore.Get()
	// Init from genesis file is done only once
	storeInitiated := state.LatestRound > 0
	// load/store unicity certificates and register partitions from root genesis file
	partitionStore, err := rootchain.NewPartitionStoreFromGenesis(g.Partitions)
	if err != nil {
		return nil, err
	}
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range g.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
		// In case the store is already initiated, check if partition identifier is known
		if storeInitiated {
			if _, f := state.Certificates[identifier]; !f {
				return nil, errors.Errorf("invalid genesis, new partition %v detected", identifier)
			}
		}
	}
	// If not initiated, save genesis file to store
	if !storeInitiated {
		if err := stateStore.Save(store.RootState{LatestRound: g.GetRoundNumber(), Certificates: certs, LatestRootHash: g.GetRoundHash()}); err != nil {
			return nil, err
		}
	}

	safety := NewSafetyModule(signer)
	// New partition manager, receives requests and responds
	p, err := NewPartitionManager(signer, pNet, stateStore, partitionStore)
	if err != nil {
		return nil, err
	}
	// Create new root validator protocol handler
	r, err := NewAtomicBroadcastManager(rootHost, configuration, stateStore, partitionStore, safety, rNet)
	if err != nil {
		return nil, err
	}

	node := &Validator{
		conf:             configuration,
		partitionHost:    prt,
		partitionStore:   partitionStore,
		partitionManager: p,
		consensusManager: r,
	}
	node.ctx, node.ctxCancel = context.WithCancel(context.Background())
	go node.loop()
	return node, nil
}

func (v *Validator) Close() {
	v.partitionManager.Timers().WaitClose()
	v.consensusManager.Timers().WaitClose()
	v.ctxCancel()
}

// loop handles messages from different goroutines.
func (v *Validator) loop() {
	for {
		select {
		case <-v.ctx.Done():
			logger.Info("Exiting root validator main loop")
			return
		case m, ok := <-v.partitionManager.Receive():
			if !ok {
				logger.Warning("Partition received channel closed, exiting root validator main loop")
				return
			}
			v.partitionManager.OnPartitionMessage(&m)
		case m, ok := <-v.consensusManager.Receive():
			if !ok {
				logger.Warning("Root network received channel closed, exiting root validator main loop")
				return
			}
			v.consensusManager.OnAtomicBroadcastMessage(&m)
		// handle timeouts
		case nt := <-v.consensusManager.Timers().C:
			if nt == nil {
				continue
			}
			timerName := nt.Name()
			v.consensusManager.OnTimeout(timerName)
		case nt := <-v.partitionManager.Timers().C:
			if nt == nil {
				continue
			}
			timerName := nt.Name()
			v.partitionManager.OnTimeout(timerName)
		}
	}
}

func loadConf(genesisRoot *genesis.GenesisRootRecord, opts []Option) *RootNodeConf {
	rootTrustBase, err := genesis.NewValidatorTrustBase(genesisRoot.RootValidators)
	if err != nil {
		return nil
	}
	nodesMap := make(map[peer.ID][]byte)
	for _, n := range genesisRoot.RootValidators {
		nodesMap[peer.ID(n.NodeIdentifier)] = n.SigningPublicKey
	}
	localTimeout := defaultLocalTimeout
	quorumThreshold := genesis.GetMinQuorumThreshold(uint32(len(nodesMap)))
	// Is consensus timeout specified?
	if genesisRoot.Consensus.ConsensusTimeoutMs != nil {
		localTimeout = time.Duration(*genesisRoot.Consensus.ConsensusTimeoutMs) * time.Millisecond
	}
	// Is consensus threshold specified
	if genesisRoot.Consensus.QuorumThreshold != nil {
		quorumThreshold = *genesisRoot.Consensus.QuorumThreshold
	}
	conf := &RootNodeConf{
		BlockRateMs:        time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeoutMs:     localTimeout,
		ConsensusThreshold: quorumThreshold,
		RootTrustBase:      rootTrustBase,
		HashAlgorithm:      gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm),
		stateStore:         store.NewInMemStateStore(gocrypto.SHA256),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}
