package testpartition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rootchain"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	NodePartitions map[types.SystemID32]*NodePartition
	RootPartition  *RootPartition
	ctxCancel      context.CancelFunc
}

type RootPartition struct {
	rcGenesis *genesis.RootGenesis
	TrustBase map[string]abcrypto.Verifier
	Nodes     []*rootNode
	obs       testobserve.Factory
}

type NodePartition struct {
	systemId         types.SystemID
	partitionGenesis *genesis.PartitionGenesis
	txSystemFunc     func(trustBase map[string]abcrypto.Verifier) txsystem.TransactionSystem
	ctx              context.Context
	tb               map[string]abcrypto.Verifier
	Nodes            []*partitionNode
	obs              testobserve.Factory
}

type partitionNode struct {
	*partition.Node
	dbFile       string
	idxFile      string
	peerConf     *network.PeerConfiguration
	signer       abcrypto.Signer
	genesis      *genesis.PartitionNode
	EventHandler *testevent.TestEventHandler
	confOpts     []partition.NodeOption
	AddrGRPC     string
	proofDB      keyvaluedb.KeyValueDB
	cancel       context.CancelFunc
	done         chan error
}

type rootNode struct {
	*rootchain.Node
	EncKeyPair *network.PeerKeyPair
	RootSigner abcrypto.Signer
	genesis    *genesis.RootGenesis
	id         peer.ID
	addr       multiaddr.Multiaddr

	cancel context.CancelFunc
	done   chan error
}

func (pn *partitionNode) Stop() error {
	pn.cancel()
	return <-pn.done
}

func (rn *rootNode) Stop() error {
	rn.cancel()
	return <-rn.done
}

const testNetworkTimeout = 600 * time.Millisecond

// getGenesisFiles is a helper function to collect all node genesis files
func getGenesisFiles(nodePartitions []*NodePartition) []*genesis.PartitionNode {
	var partitionRecords []*genesis.PartitionNode
	for _, part := range nodePartitions {
		for _, node := range part.Nodes {
			partitionRecords = append(partitionRecords, node.genesis)
		}
	}
	return partitionRecords
}

// newRootPartition creates new root partition, requires node partitions with genesis files
func newRootPartition(nofRootNodes uint8, nodePartitions []*NodePartition) (*RootPartition, error) {
	encKeyPairs, err := generateKeyPairs(nofRootNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption keypairs, %w", err)
	}
	rootSigners, err := createSigners(nofRootNodes)
	if err != nil {
		return nil, fmt.Errorf("create signer failed, %w", err)
	}
	trustBase := make(map[string]abcrypto.Verifier)
	rootNodes := make([]*rootNode, nofRootNodes)
	rootGenesisFiles := make([]*genesis.RootGenesis, nofRootNodes)
	for i := 0; i < int(nofRootNodes); i++ {
		encPubKey, err := libp2pcrypto.UnmarshalSecp256k1PublicKey(encKeyPairs[i].PublicKey)
		if err != nil {
			return nil, err
		}
		pubKeyBytes, err := encPubKey.Raw()
		if err != nil {
			return nil, err
		}
		id, err := peer.IDFromPublicKey(encPubKey)
		if err != nil {
			return nil, fmt.Errorf("root node id error, %w", err)
		}
		nodeGenesisFiles := getGenesisFiles(nodePartitions)
		pr, err := rootgenesis.NewPartitionRecordFromNodes(nodeGenesisFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to create genesis partition record")
		}
		rg, _, err := rootgenesis.NewRootGenesis(
			id.String(),
			rootSigners[i],
			pubKeyBytes,
			pr,
			rootgenesis.WithTotalNodes(uint32(nofRootNodes)),
			rootgenesis.WithBlockRate(genesis.MinBlockRateMs),
			rootgenesis.WithConsensusTimeout(genesis.DefaultConsensusTimeout))
		if err != nil {
			return nil, err
		}
		ver, err := rootSigners[i].Verifier()
		if err != nil {
			return nil, fmt.Errorf("failed to get root node verifier, %w", err)
		}
		trustBase[id.String()] = ver
		rootGenesisFiles[i] = rg
		rootNodes[i] = &rootNode{
			genesis:    rg,
			RootSigner: rootSigners[i],
			EncKeyPair: encKeyPairs[i],
		}
	}
	rootGenesis, partitionGenesisFiles, err := rootgenesis.MergeRootGenesisFiles(rootGenesisFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to finalize root genesis, %w", err)
	}
	// update partition genesis files
	for _, pg := range partitionGenesisFiles {
		for _, part := range nodePartitions {
			if bytes.Equal(part.systemId, pg.SystemDescriptionRecord.SystemIdentifier) {
				part.partitionGenesis = pg
			}
		}
	}
	return &RootPartition{
		rcGenesis: rootGenesis,
		TrustBase: trustBase,
		Nodes:     rootNodes,
	}, nil
}

func (r *RootPartition) start(ctx context.Context) error {
	rootNodes := len(r.Nodes)
	var peerIDs = make([]peer.ID, rootNodes)
	for i := 0; i < len(peerIDs); i++ {
		id, err := network.NodeIDFromPublicKeyBytes(r.Nodes[i].EncKeyPair.PublicKey)
		if err != nil {
			return fmt.Errorf("peer id from public key failed: %w", err)
		}
		peerIDs[i] = id
	}
	var rootPeers = make([]*network.Peer, rootNodes)
	for i := 0; i < len(peerIDs); i++ {
		port, err := net.GetFreePort()
		if err != nil {
			return fmt.Errorf("failed to get free port, %w", err)
		}
		peerConf, err := network.NewPeerConfiguration(fmt.Sprintf("/ip4/127.0.0.1/tcp/%v", port), r.Nodes[i].EncKeyPair, nil, peerIDs)
		if err != nil {
			return fmt.Errorf("failed to create peer configuration: %w", err)
		}
		rootPeers[i], err = network.NewPeer(ctx, peerConf, r.obs.DefaultLogger(), nil)
		if err != nil {
			return fmt.Errorf("failed to create root peer node: %w", err)
		}
	}
	// start root nodes
	for i, rn := range r.Nodes {
		rootPeer := rootPeers[i]
		log := r.obs.DefaultLogger().With(logger.NodeID(rootPeer.ID()))
		obs := observability.WithLogger(r.obs.DefaultObserver(), log)
		// this is a unit test set-up pre-populate store with addresses, create separate test for node discovery
		for _, p := range rootPeers {
			rootPeer.Network().Peerstore().AddAddr(p.ID(), p.MultiAddresses()[0], peerstore.PermanentAddrTTL)
		}
		rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init root and partition nodes network, %w", err)
		}
		// Initiate partition store
		partitionStore, err := partitions.NewPartitionStoreFromGenesis(r.rcGenesis.Partitions)
		if err != nil {
			return fmt.Errorf("failed to create partition store form root genesis, %w", err)
		}
		rootConsensusNet, err := network.NewLibP2RootConsensusNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init consensus network, %w", err)
		}
		cm, err := abdrc.NewDistributedAbConsensusManager(rootPeer.ID(), r.rcGenesis, partitionStore, rootConsensusNet, rn.RootSigner, obs)
		if err != nil {
			return fmt.Errorf("consensus manager initialization failed, %w", err)
		}
		rootchainNode, err := rootchain.New(rootPeer, rootNet, partitionStore, cm, obs)
		if err != nil {
			return fmt.Errorf("failed to create root node, %w", err)
		}
		rn.Node = rootchainNode
		rn.addr = rootPeers[i].MultiAddresses()[0]
		rn.id = rootPeer.ID()
		// start root node
		nctx, ncfn := context.WithCancel(ctx)
		rn.cancel = ncfn
		rn.done = make(chan error, 1)
		// start root node
		go func(ec chan error) { ec <- rootchainNode.Run(nctx) }(rn.done)
	}
	return nil
}

func NewPartition(t *testing.T, nodeCount uint8, txSystemProvider func(trustBase map[string]abcrypto.Verifier) txsystem.TransactionSystem, systemIdentifier []byte) (abPartition *NodePartition, err error) {
	if nodeCount < 1 {
		return nil, fmt.Errorf("invalid count of partition Nodes: %d", nodeCount)
	}
	abPartition = &NodePartition{
		systemId:     systemIdentifier,
		txSystemFunc: txSystemProvider,
		Nodes:        make([]*partitionNode, nodeCount),
		obs:          testobserve.NewFactory(t),
	}
	// create peer configurations
	peerConfs, err := createPeerConfs(nodeCount)
	if err != nil {
		return nil, err
	}
	// create partition signing keys
	signers, err := createSigners(nodeCount)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(nodeCount); i++ {
		peerConf := peerConfs[i]

		signer := signers[i]
		// create partition genesis file
		nodeGenesis, err := partition.NewNodeGenesis(
			txSystemProvider(map[string]abcrypto.Verifier{"genesis": nil}),
			partition.WithPeerID(peerConf.ID),
			partition.WithSigningKey(signer),
			partition.WithEncryptionPubKey(peerConf.KeyPair.PublicKey),
			partition.WithSystemIdentifier(systemIdentifier),
			partition.WithT2Timeout(2500),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create node genesis, %w", err)
		}
		tmpDir := t.TempDir()
		abPartition.Nodes[i] = &partitionNode{
			genesis:  nodeGenesis,
			peerConf: peerConf,
			signer:   signer,
			dbFile:   filepath.Join(tmpDir, "blocks.db"),
			idxFile:  filepath.Join(tmpDir, "tx.db"),
		}
	}
	return abPartition, nil
}

func (n *NodePartition) start(t *testing.T, ctx context.Context, bootNodes []peer.AddrInfo) error {
	n.ctx = ctx
	// start Nodes
	trustBase, err := genesis.NewValidatorTrustBase(n.partitionGenesis.RootValidators)
	if err != nil {
		return fmt.Errorf("failed to extract root trust base from genesis file, %w", err)
	}
	n.tb = trustBase

	for _, nd := range n.Nodes {
		nd.EventHandler = &testevent.TestEventHandler{}
		blockStore, err := boltdb.New(nd.dbFile)
		if err != nil {
			return err
		}
		t.Cleanup(func() { require.NoError(t, blockStore.Close()) })
		nd.proofDB = memorydb.New()
		// set root node as bootstrap peer
		nd.peerConf.BootstrapPeers = bootNodes
		nd.confOpts = append(nd.confOpts,
			partition.WithEventHandler(nd.EventHandler.HandleEvent, 100),
			partition.WithBlockStore(blockStore),
			partition.WithProofIndex(nd.proofDB, 0),
		)
		if err = n.startNode(ctx, nd); err != nil {
			return err
		}
	}
	// make sure node network (to other nodes and root nodes) is initiated
	for _, nd := range n.Nodes {
		if ok := eventually(
			func() bool { return len(nd.GetPeer().Network().Peers()) >= len(n.Nodes) },
			2*time.Second, 100*time.Millisecond); !ok {
			return fmt.Errorf("network not initialized")
		}
	}
	return nil
}

func (n *NodePartition) startNode(ctx context.Context, pn *partitionNode) error {
	log := n.obs.DefaultLogger().With(logger.NodeID(pn.peerConf.ID))
	node, err := partition.NewNode(
		ctx,
		pn.peerConf,
		pn.signer,
		n.txSystemFunc(n.tb),
		n.partitionGenesis,
		nil,
		observability.WithLogger(n.obs.DefaultObserver(), log),
		pn.confOpts...,
	)
	if err != nil {
		return fmt.Errorf("failed to resume node, %w", err)
	}
	nctx, ncfn := context.WithCancel(n.ctx)
	pn.Node = node
	pn.cancel = ncfn
	pn.done = make(chan error, 1)
	go func(ec chan error) { ec <- node.Run(nctx) }(pn.done)
	return nil
}

func NewAlphabillPartition(nodePartitions []*NodePartition) (*AlphabillNetwork, error) {
	if len(nodePartitions) < 1 {
		return nil, fmt.Errorf("no node partitions set, it makes no sense to start with only root")
	}
	// create root node(s)
	rootPartition, err := newRootPartition(1, nodePartitions)
	if err != nil {
		return nil, err
	}
	nodeParts := make(map[types.SystemID32]*NodePartition)
	for _, part := range nodePartitions {
		sysID, _ := part.systemId.Id32()
		nodeParts[sysID] = part
	}
	return &AlphabillNetwork{
		RootPartition:  rootPartition,
		NodePartitions: nodeParts,
	}, nil
}

func NewMultiRootAlphabillPartition(nofRootNodes uint8, nodePartitions []*NodePartition) (*AlphabillNetwork, error) {
	if len(nodePartitions) < 1 {
		return nil, fmt.Errorf("no node partitions set, it makes no sense to start with only root")
	}
	// create root node(s)
	rootPartition, err := newRootPartition(nofRootNodes, nodePartitions)
	if err != nil {
		return nil, err
	}
	nodeParts := make(map[types.SystemID32]*NodePartition)
	for _, part := range nodePartitions {
		sysID, _ := part.systemId.Id32()
		nodeParts[sysID] = part
	}
	return &AlphabillNetwork{
		RootPartition:  rootPartition,
		NodePartitions: nodeParts,
	}, nil
}

func getBootstrapNodes(t *testing.T, root *RootPartition) []peer.AddrInfo {
	require.NotNil(t, root)
	bootNodes := make([]peer.AddrInfo, len(root.Nodes))
	for i, n := range root.Nodes {
		bootNodes[i] = peer.AddrInfo{ID: n.id, Addrs: []multiaddr.Multiaddr{n.addr}}
	}
	return bootNodes
}

func (a *AlphabillNetwork) Start(t *testing.T) error {
	a.RootPartition.obs = testobserve.NewFactory(t)
	// create context
	ctx, ctxCancel := context.WithCancel(context.Background())
	if err := a.RootPartition.start(ctx); err != nil {
		ctxCancel()
		return fmt.Errorf("failed to start root partition, %w", err)
	}
	bootNodes := getBootstrapNodes(t, a.RootPartition)
	for id, part := range a.NodePartitions {
		// create one event handler per partition
		if err := part.start(t, ctx, bootNodes); err != nil {
			ctxCancel()
			return fmt.Errorf("failed to start partition %s, %w", id, err)
		}
	}
	a.ctxCancel = ctxCancel
	return nil
}

func (a *AlphabillNetwork) Close() (err error) {
	a.ctxCancel()
	// wait and check validator exit
	for _, part := range a.NodePartitions {
		// stop all nodes
		for _, n := range part.Nodes {
			nodeErr := <-n.done
			if !errors.Is(nodeErr, context.Canceled) {
				err = errors.Join(err, nodeErr)
			}
		}
	}
	// check root exit
	for _, rnode := range a.RootPartition.Nodes {
		rootErr := <-rnode.done
		if !errors.Is(rootErr, context.Canceled) {
			err = errors.Join(err, rootErr)
		}
	}
	return err
}

/*
WaitClose closes the AB network and waits for all the nodes to stop.
It fails the test "t" if nodes do not stop/exit within timeout.
*/
func (a *AlphabillNetwork) WaitClose(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.Close(); err != nil {
			t.Errorf("stopping AB network: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(3000 * time.Millisecond):
		t.Error("AB network didn't stop within timeout")
	}
}

func (a *AlphabillNetwork) GetNodePartition(sysID types.SystemID) (*NodePartition, error) {
	id, _ := sysID.Id32()
	part, f := a.NodePartitions[id]
	if !f {
		return nil, fmt.Errorf("unknown partition %X", sysID)
	}
	return part, nil
}

func (a *AlphabillNetwork) GetValidator(sysID types.SystemID) (partition.UnicityCertificateValidator, error) {
	id, _ := sysID.Id32()
	part, f := a.NodePartitions[id]
	if !f {
		return nil, fmt.Errorf("unknown partition %X", sysID)
	}
	return partition.NewDefaultUnicityCertificateValidator(part.partitionGenesis.SystemDescriptionRecord, a.RootPartition.TrustBase, gocrypto.SHA256)
}

// BroadcastTx sends transactions to all nodes.
func (n *NodePartition) BroadcastTx(tx *types.TransactionOrder) error {
	for _, n := range n.Nodes {
		if _, err := n.SubmitTx(context.Background(), tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (n *NodePartition) SubmitTx(tx *types.TransactionOrder) error {
	_, err := n.Nodes[0].SubmitTx(context.Background(), tx)
	return err
}

func (n *NodePartition) GetTxProof(tx *types.TransactionOrder) (*types.Block, *types.TxProof, *types.TransactionRecord, error) {
	for _, n := range n.Nodes {
		bl, err := n.GetLatestBlock()
		if err != nil {
			return nil, nil, nil, err
		}
		number := bl.UnicityCertificate.InputRecord.RoundNumber
		for i := uint64(0); i < number; i++ {
			b, err := n.GetBlock(context.Background(), number-i)
			if err != nil || b == nil {
				continue
			}
			for j, t := range b.Transactions {
				if reflect.DeepEqual(t.TransactionOrder, tx) {
					proof, _, err := types.NewTxProof(b, j, gocrypto.SHA256)
					if err != nil {
						return nil, nil, nil, err
					}
					return b, proof, t, nil
				}
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("tx with id %x was not found", tx.UnitID())
}

// WaitTxProof - wait for proof from any validator in partition. If one has the proof it does not mean all have processed
// the UC. Returns both transaction record and proof when tx has been executed and added to block
func WaitTxProof(t *testing.T, part *NodePartition, txOrder *types.TransactionOrder) (*types.TransactionRecord, *types.TxProof, error) {
	t.Helper()
	var (
		txRecord *types.TransactionRecord
		txProof  *types.TxProof
	)
	txHash := txOrder.Hash(gocrypto.SHA256)
	if ok := eventually(func() bool {
		for _, n := range part.Nodes {
			txRec, proof, err := n.GetTransactionRecord(context.Background(), txHash)
			if errors.Is(err, partition.ErrIndexNotFound) {
				continue
			}
			txRecord = txRec
			txProof = proof
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick); ok {
		return txRecord, txProof, nil
	}
	return nil, nil, fmt.Errorf("failed to confirm tx")
}

func WaitUnitProof(t *testing.T, part *NodePartition, ID types.UnitID, txOrder *types.TransactionOrder) (*types.UnitDataAndProof, error) {
	t.Helper()
	var (
		unitProof *types.UnitDataAndProof
	)
	txOrderHash := txOrder.Hash(gocrypto.SHA256)
	if ok := eventually(func() bool {
		for _, n := range part.Nodes {
			unitDataAndProof, err := partition.ReadUnitProofIndex(n.proofDB, ID, txOrderHash)
			if err != nil {
				continue
			}
			unitProof = unitDataAndProof
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick); ok {
		return unitProof, nil
	}
	return nil, fmt.Errorf("failed to confirm tx")
}

// BlockchainContainsTx checks if at least one partition node block contains the given transaction.
func BlockchainContainsTx(part *NodePartition, tx *types.TransactionOrder) func() bool {
	return BlockchainContains(part, func(actualTx *types.TransactionOrder) bool {
		return reflect.DeepEqual(actualTx, tx)
	})
}

func BlockchainContains(part *NodePartition, criteria func(tx *types.TransactionOrder) bool) func() bool {
	return func() bool {
		for _, n := range part.Nodes {
			bl, err := n.GetLatestBlock()
			if err != nil {
				panic(err)
			}
			number := bl.UnicityCertificate.InputRecord.RoundNumber
			for i := uint64(0); i <= number; i++ {
				b, err := n.GetBlock(context.Background(), number-i)
				if err != nil || b == nil {
					continue
				}
				for _, t := range b.Transactions {
					if criteria(t.TransactionOrder) {
						return true
					}
				}
			}
		}
		return false
	}
}

func createSigners(count uint8) ([]abcrypto.Signer, error) {
	var signers = make([]abcrypto.Signer, count)
	for i := 0; i < int(count); i++ {
		s, err := abcrypto.NewInMemorySecp256K1Signer()
		if err != nil {
			return nil, err
		}
		signers[i] = s
	}
	return signers, nil
}

func createPeerConfs(count uint8) ([]*network.PeerConfiguration, error) {
	var peerConfs = make([]*network.PeerConfiguration, count)

	// generate connection encryption key pairs
	keyPairs, err := generateKeyPairs(count)
	if err != nil {
		return nil, err
	}

	var validators = make(peer.IDSlice, count)

	for i := 0; i < int(count); i++ {
		peerConfs[i], err = network.NewPeerConfiguration(
			"/ip4/127.0.0.1/tcp/0",
			keyPairs[i], // connection encryption key. The ID of the node is derived from this keypair.
			nil,
			validators, // Persistent peers
		)
		if err != nil {
			return nil, err
		}
		validators[i] = peerConfs[i].ID
	}
	sort.Sort(validators)

	return peerConfs, nil
}

func generateKeyPairs(count uint8) ([]*network.PeerKeyPair, error) {
	var keyPairs = make([]*network.PeerKeyPair, count)
	for i := 0; i < int(count); i++ {
		privateKey, publicKey, err := libp2pcrypto.GenerateSecp256k1Key(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate key pair %d/%d: %w", i, count, err)
		}
		privateKeyBytes, err := privateKey.Raw()
		if err != nil {
			return nil, err
		}
		pubKeyBytes, err := publicKey.Raw()
		if err != nil {
			return nil, err
		}
		keyPairs[i] = &network.PeerKeyPair{
			PublicKey:  pubKeyBytes,
			PrivateKey: privateKeyBytes,
		}
		if err != nil {
			return nil, err
		}
	}
	return keyPairs, nil
}

func eventually(condition func() bool, waitFor time.Duration, tick time.Duration) bool {
	ch := make(chan bool, 1)

	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			return false
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case v := <-ch:
			if v {
				return true
			}
			tick = ticker.C
		}
	}
}
