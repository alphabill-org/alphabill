package testpartition

import (
	"bytes"
	"cmp"
	"context"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"sort"
	"testing"
	"time"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
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
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	NodePartitions map[types.PartitionID]*NodePartition
	RootPartition  *RootPartition
	BootNodes      []*network.Peer
	ctxCancel      context.CancelFunc
}

type RootPartition struct {
	rcGenesis *genesis.RootGenesis
	TrustBase types.RootTrustBase
	Nodes     []*rootNode
	obs       testobserve.Factory
}

type NodePartition struct {
	partitionID      types.PartitionID
	partitionGenesis *genesis.PartitionGenesis
	genesisState     *state.State
	txSystemFunc     func(trustBase types.RootTrustBase) txsystem.TransactionSystem
	ctx              context.Context
	tb               types.RootTrustBase
	Nodes            []*partitionNode
	obs              testobserve.Factory
}

type partitionNode struct {
	*partition.Node
	dbFile            string
	idxFile           string
	orchestrationFile string
	peerConf          *network.PeerConfiguration
	signer            abcrypto.Signer
	genesis           *genesis.PartitionNode
	EventHandler      *testevent.TestEventHandler
	confOpts          []partition.NodeOption
	proofDB           keyvaluedb.KeyValueDB
	cancel            context.CancelFunc
	done              chan error
}

type rootNode struct {
	*rootchain.Node
	RootSigner abcrypto.Signer
	genesis    *genesis.RootGenesis
	peerConf   *network.PeerConfiguration
	addr       []multiaddr.Multiaddr
	homeDir    string

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
func newRootPartition(t *testing.T, nofRootNodes uint8, nodePartitions []*NodePartition) (*RootPartition, error) {
	rootSigners, err := createSigners(nofRootNodes)
	if err != nil {
		return nil, fmt.Errorf("create signer failed, %w", err)
	}
	rootNodes := make([]*rootNode, nofRootNodes)
	rootGenesisFiles := make([]*genesis.RootGenesis, nofRootNodes)
	trustBaseNodes := make([]*types.NodeInfo, nofRootNodes)
	var unicityTreeRootHash []byte
	// generates keys and sorts them in lexical order - meaning root node 0 is the first leader
	rootPeerCfg, _, err := createPeerConfs(nofRootNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate peer configuration, %w", err)
	}
	for i, peerCfg := range rootPeerCfg {
		nodes := getGenesisFiles(nodePartitions)
		rg, _, err := rootgenesis.NewRootGenesis(
			peerCfg.ID.String(),
			rootSigners[i],
			peerCfg.KeyPair.PublicKey,
			nodes,
			rootgenesis.WithTotalNodes(uint32(nofRootNodes)),
			rootgenesis.WithBlockRate(max(genesis.MinBlockRateMs, 300)), // set block rate at 300 ms to give a bit more time for nodes to bootstrap
			rootgenesis.WithConsensusTimeout(genesis.DefaultConsensusTimeout))
		if err != nil {
			return nil, err
		}
		verifier, err := rootSigners[i].Verifier()
		if err != nil {
			return nil, fmt.Errorf("failed to get root node verifier, %w", err)
		}
		rootGenesisFiles[i] = rg
		rootNodes[i] = &rootNode{
			genesis:    rg,
			RootSigner: rootSigners[i],
			peerConf:   peerCfg,
			homeDir:    t.TempDir(),
		}
		trustBaseNodes[i] = types.NewNodeInfo(peerCfg.ID.String(), 1, verifier)
		for _, p := range rg.Partitions {
			if len(unicityTreeRootHash) == 0 {
				unicityTreeRootHash = p.Certificate.UnicitySeal.Hash
			} else if !bytes.Equal(unicityTreeRootHash, p.Certificate.UnicitySeal.Hash) {
				return nil, errors.New("unicity certificate seal hashes are not equal")
			}
		}
	}
	trustBase, err := types.NewTrustBaseGenesis(trustBaseNodes, unicityTreeRootHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis trust base, %w", err)
	}

	rootGenesis, partitionGenesisFiles, err := rootgenesis.MergeRootGenesisFiles(rootGenesisFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to finalize root genesis, %w", err)
	}
	// update partition genesis files
	for _, pg := range partitionGenesisFiles {
		for _, part := range nodePartitions {
			if part.partitionID == pg.PartitionDescription.PartitionID {
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

func (r *RootPartition) start(ctx context.Context, bootNodes []peer.AddrInfo, rootNodes ...*rootNode) error {
	// start root nodes
	for _, rn := range rootNodes {
		log := r.obs.DefaultLogger().With(logger.NodeID(rn.peerConf.ID))
		obs := observability.WithLogger(r.obs.DefaultObserver(), log)
		if len(bootNodes) > 0 {
			rn.peerConf.BootstrapPeers = bootNodes
		}
		rootPeer, err := network.NewPeer(ctx, rn.peerConf, log, nil)
		if err != nil {
			return fmt.Errorf("failed to create root peer node: %w", err)
		}
		if len(bootNodes) > 0 {
			if err = rootPeer.BootstrapConnect(ctx, log); err != nil {
				return fmt.Errorf("root node bootstrap failed, %w", err)
			}
		}
		rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init root and partition nodes network, %w", err)
		}

		rootConsensusNet, err := network.NewLibP2RootConsensusNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init consensus network, %w", err)
		}

		orchestration, err := partitions.NewOrchestration(r.rcGenesis, filepath.Join(rn.homeDir, "orchestration.db"))
		if err != nil {
			return fmt.Errorf("failed to init orchestration: %w", err)
		}
		cm, err := consensus.NewConsensusManager(rootPeer.ID(), r.rcGenesis, r.TrustBase, orchestration, rootConsensusNet, rn.RootSigner, obs)
		if err != nil {
			return fmt.Errorf("consensus manager initialization failed, %w", err)
		}
		rootchainNode, err := rootchain.New(rootPeer, rootNet, cm, obs)
		if err != nil {
			return fmt.Errorf("failed to create root node, %w", err)
		}
		rn.Node = rootchainNode
		rn.addr = rootPeer.MultiAddresses()
		// start root node
		nctx, ncfn := context.WithCancel(ctx)
		rn.cancel = ncfn
		rn.done = make(chan error, 1)
		// start root node
		go func(ec chan error) { ec <- rootchainNode.Run(nctx) }(rn.done)
	}
	return nil
}

func NewPartition(t *testing.T, nodeCount uint8, txSystemProvider func(trustBase types.RootTrustBase) txsystem.TransactionSystem, pdr types.PartitionDescriptionRecord, state *state.State) (abPartition *NodePartition, err error) {
	if nodeCount < 1 {
		return nil, fmt.Errorf("invalid count of partition Nodes: %d", nodeCount)
	}
	abPartition = &NodePartition{
		partitionID:  pdr.PartitionID,
		txSystemFunc: txSystemProvider,
		genesisState: state,
		Nodes:        make([]*partitionNode, nodeCount),
		obs:          testobserve.NewFactory(t),
	}
	// create peer configurations
	peerConfs, _, err := createPeerConfs(nodeCount)
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
			state,
			pdr,
			partition.WithPeerID(peerConf.ID),
			partition.WithSignPrivKey(signer),
			partition.WithAuthPubKey(peerConf.KeyPair.PublicKey),
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
	// partition nodes as they are set up for test, require bootstrap info
	require.NotEmpty(t, bootNodes)
	// start Nodes
	trustBase, err := n.partitionGenesis.GenerateRootTrustBase()
	if err != nil {
		return fmt.Errorf("failed to extract root trust base from genesis file, %w", err)
	}
	n.tb = trustBase

	if !n.genesisState.IsCommitted() {
		if err := n.genesisState.Commit(n.partitionGenesis.Certificate); err != nil {
			return fmt.Errorf("invalid genesis state: %w", err)
		}
	}

	for _, nd := range n.Nodes {
		nd.EventHandler = &testevent.TestEventHandler{}
		blockStore, err := boltdb.New(nd.dbFile)
		if err != nil {
			return err
		}
		t.Cleanup(func() { require.NoError(t, blockStore.Close()) })
		if nd.proofDB, err = memorydb.New(); err != nil {
			return fmt.Errorf("creating proof DB: %w", err)
		}
		// set root node as bootstrap peer
		nd.peerConf.BootstrapPeers = bootNodes
		nd.confOpts = append(nd.confOpts,
			partition.WithEventHandler(nd.EventHandler.HandleEvent, 100),
			partition.WithBlockStore(blockStore),
			partition.WithProofIndex(nd.proofDB, 0),
			partition.WithOwnerIndex(partition.NewOwnerIndexer(n.obs.DefaultLogger())),
		)
		if err = n.startNode(ctx, nd); err != nil {
			return err
		}
	}
	// make sure node network (to other nodes and root nodes) is initiated
	for _, nd := range n.Nodes {
		if ok := test.Eventually(
			func() bool { return len(nd.Peer().Network().Peers()) >= len(n.Nodes) },
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
		n.tb,
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

func NewAlphabillPartition(t *testing.T, nodePartitions []*NodePartition) (*AlphabillNetwork, error) {
	if len(nodePartitions) < 1 {
		return nil, fmt.Errorf("no node partitions set, it makes no sense to start with only root")
	}
	// create root node(s)
	rootPartition, err := newRootPartition(t, 1, nodePartitions)
	if err != nil {
		return nil, err
	}
	nodeParts := make(map[types.PartitionID]*NodePartition)
	for _, part := range nodePartitions {
		nodeParts[part.partitionID] = part
	}
	return &AlphabillNetwork{
		RootPartition:  rootPartition,
		NodePartitions: nodeParts,
	}, nil
}

func NewMultiRootAlphabillPartition(t *testing.T, nofRootNodes uint8, nodePartitions []*NodePartition) (*AlphabillNetwork, error) {
	if len(nodePartitions) < 1 {
		return nil, fmt.Errorf("no node partitions set, it makes no sense to start with only root")
	}
	// create root node(s)
	rootPartition, err := newRootPartition(t, nofRootNodes, nodePartitions)
	if err != nil {
		return nil, err
	}
	nodeParts := make(map[types.PartitionID]*NodePartition)
	for _, part := range nodePartitions {
		nodeParts[part.partitionID] = part
	}
	return &AlphabillNetwork{
		RootPartition:  rootPartition,
		NodePartitions: nodeParts,
	}, nil
}

func (a *AlphabillNetwork) createBootNodes(t *testing.T, ctx context.Context, obs testobserve.Factory, nofBootNodes uint8) {
	encKeyPairs, err := generateKeyPairs(nofBootNodes)
	require.NoError(t, err)
	bootNodes := make([]*network.Peer, nofBootNodes)
	for i := 0; i < int(nofBootNodes); i++ {
		peerConf, err := network.NewPeerConfiguration("/ip4/127.0.0.1/tcp/0", nil, encKeyPairs[i], nil)
		require.NoError(t, err)
		bootNodes[i], err = network.NewPeer(ctx, peerConf, obs.DefaultLogger(), nil)
		require.NoError(t, err)
	}
	a.BootNodes = bootNodes
}

// Start AB network, no bootstrap all id's and addresses are injected to peer store at start
func (a *AlphabillNetwork) Start(t *testing.T) error {
	a.RootPartition.obs = testobserve.NewFactory(t)
	// create context
	ctx, ctxCancel := context.WithCancel(context.Background())
	// by default, use root node 1 as bootstrap
	require.NotEmpty(t, a.RootPartition.Nodes)
	// sort by node id to make sure that leader is not first node (if not only 1 node)
	// first leader is selected by round number from lexically sorted ID's
	// NB! this is not an optimal solution, but it is a unit test issue, in a real deploy
	// it is ok that some rounds at start are lost until enough nodes are up.
	slices.SortFunc(a.RootPartition.Nodes, func(a, b *rootNode) int {
		return cmp.Compare(a.peerConf.ID, b.peerConf.ID)
	})
	// first start root node 0 and use it as bootstrap node
	bootNode, rest := a.RootPartition.Nodes[0], a.RootPartition.Nodes[1:]
	if err := a.RootPartition.start(ctx, nil, bootNode); err != nil {
		ctxCancel()
		return fmt.Errorf("failed to start root partition boot node, %w", err)
	}
	bootStrapInfo := []peer.AddrInfo{{
		ID:    a.RootPartition.Nodes[0].peerConf.ID,
		Addrs: a.RootPartition.Nodes[0].addr,
	}}
	if len(rest) > 0 {
		if err := a.RootPartition.start(ctx, bootStrapInfo, rest...); err != nil {
			ctxCancel()
			return fmt.Errorf("failed to start root partition, %w", err)
		}
	}
	for id, part := range a.NodePartitions {
		// create one event handler per partition
		if err := part.start(t, ctx, bootStrapInfo); err != nil {
			ctxCancel()
			return fmt.Errorf("failed to start partition %s, %w", id, err)
		}
	}
	a.ctxCancel = ctxCancel
	return nil
}

// StartWithStandAloneBootstrapNodes - starts AB network with dedicated bootstrap nodes
func (a *AlphabillNetwork) StartWithStandAloneBootstrapNodes(t *testing.T) error {
	a.RootPartition.obs = testobserve.NewFactory(t)
	// create context
	ctx, ctxCancel := context.WithCancel(context.Background())
	// create boot nodes
	a.createBootNodes(t, ctx, a.RootPartition.obs, 2)
	// Setup bootstrap info for the whole network to use the dedicated nodes
	bootStrapInfo := make([]peer.AddrInfo, len(a.BootNodes))
	for i, n := range a.BootNodes {
		bootStrapInfo[i] = peer.AddrInfo{ID: n.ID(), Addrs: n.MultiAddresses()}
	}
	// sort by node id to make sure that leader is not first node (if not only 1 node)
	// first leader is selected by round number from lexically sorted ID's
	// NB! this is not an optimal solution, but it is a unit test issue, in a real deploy
	// it is ok that some rounds at start are lost until enough nodes are up.
	slices.SortFunc(a.RootPartition.Nodes, func(a, b *rootNode) int {
		return cmp.Compare(a.peerConf.ID, b.peerConf.ID)
	})
	if err := a.RootPartition.start(ctx, bootStrapInfo, a.RootPartition.Nodes...); err != nil {
		ctxCancel()
		return fmt.Errorf("failed to start root partition with dedicated boot node, %w", err)
	}
	for id, part := range a.NodePartitions {
		// create one event handler per partition
		if err := part.start(t, ctx, bootStrapInfo); err != nil {
			ctxCancel()
			return fmt.Errorf("failed to start partition %s, %w", id, err)
		}
	}
	a.ctxCancel = ctxCancel
	return nil
}

func (a *AlphabillNetwork) Close() (retErr error) {
	a.ctxCancel()
	// wait and check validator exit
	for _, part := range a.NodePartitions {
		// stop all nodes
		for _, n := range part.Nodes {
			if err := n.Node.Peer().Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("peer close error: %w", err))
			}
			nodeErr := <-n.done
			if !errors.Is(nodeErr, context.Canceled) {
				retErr = errors.Join(retErr, nodeErr)
			}
		}
	}
	// check root exit
	for _, rNode := range a.RootPartition.Nodes {
		if err := rNode.Node.GetPeer().Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("peer close error: %w", err))
		}
		rootErr := <-rNode.done
		if !errors.Is(rootErr, context.Canceled) {
			retErr = errors.Join(retErr, rootErr)
		}
	}
	return retErr
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
	case <-time.After(10 * time.Second):
		_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		t.Error("AB network didn't stop within timeout")
	}
}

func (a *AlphabillNetwork) GetNodePartition(sysID types.PartitionID) (*NodePartition, error) {
	part, f := a.NodePartitions[sysID]
	if !f {
		return nil, fmt.Errorf("unknown partition %s", sysID)
	}
	return part, nil
}

func (a *AlphabillNetwork) GetValidator(sysID types.PartitionID) (partition.UnicityCertificateValidator, error) {
	part, f := a.NodePartitions[sysID]
	if !f {
		return nil, fmt.Errorf("unknown partition %s", sysID)
	}
	return partition.NewDefaultUnicityCertificateValidator(part.partitionGenesis.PartitionDescription, a.RootPartition.TrustBase, crypto.SHA256)
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

func (n *NodePartition) GetTxProof(t *testing.T, tx *types.TransactionOrder) (*types.Block, *types.TxRecordProof, error) {
	txBytes := testtransaction.TxoToBytes(t, tx)
	for _, n := range n.Nodes {
		number, err := n.LatestBlockNumber()
		if err != nil {
			return nil, nil, err
		}
		for i := uint64(0); i < number; i++ {
			b, err := n.GetBlock(context.Background(), number-i)
			if err != nil || b == nil {
				continue
			}
			for j, t := range b.Transactions {
				if bytes.Equal(t.TransactionOrder, txBytes) {
					proof, err := types.NewTxRecordProof(b, j, crypto.SHA256)
					if err != nil {
						return nil, nil, err
					}
					return b, proof, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("tx with id %x was not found", tx.UnitID)
}

// PartitionInitReady - all nodes are in normal state and return the latest block number
func PartitionInitReady(t *testing.T, part *NodePartition) func() bool {
	t.Helper()
	return func() bool {
		for _, n := range part.Nodes {
			_, err := n.LatestBlockNumber()
			if err != nil {
				return false
			}
		}
		return true
	}
}

// WaitTxProof - wait for proof from any validator in partition. If one has the proof it does not mean all have processed
// the UC. Returns both transaction record and proof when tx has been executed and added to block
func WaitTxProof(t *testing.T, part *NodePartition, txOrder *types.TransactionOrder) (*types.TxRecordProof, error) {
	t.Helper()
	var txRecordProof *types.TxRecordProof
	txHash := test.DoHash(t, txOrder)
	ok := test.Eventually(func() bool {
		for _, n := range part.Nodes {
			txRecProof, err := n.GetTransactionRecordProof(context.Background(), txHash)
			if errors.Is(err, partition.ErrIndexNotFound) {
				continue
			}
			txRecordProof = txRecProof
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	if !ok {
		return nil, fmt.Errorf("failed to confirm tx")
	}
	return txRecordProof, nil
}

func WaitUnitProof(t *testing.T, part *NodePartition, ID types.UnitID, txOrder *types.TransactionOrder) (*types.UnitDataAndProof, error) {
	t.Helper()
	var (
		unitProof *types.UnitDataAndProof
	)
	txHash := test.DoHash(t, txOrder)
	if ok := test.Eventually(func() bool {
		for _, n := range part.Nodes {
			unitDataAndProof, err := partition.ReadUnitProofIndex(n.proofDB, ID, txHash)
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
func BlockchainContainsTx(t *testing.T, part *NodePartition, tx *types.TransactionOrder) func() bool {
	return BlockchainContains(t, part, func(actualTx *types.TransactionRecord) bool {
		return bytes.Equal(actualTx.TransactionOrder, testtransaction.TxoToBytes(t, tx))
	})
}

// BlockchainContainsSuccessfulTx checks if at least one partition node has successfully executed the given transaction.
func BlockchainContainsSuccessfulTx(t *testing.T, part *NodePartition, tx *types.TransactionOrder) func() bool {
	return BlockchainContains(t, part, func(actualTx *types.TransactionRecord) bool {
		return actualTx.ServerMetadata.SuccessIndicator == types.TxStatusSuccessful &&
			bytes.Equal(actualTx.TransactionOrder, testtransaction.TxoToBytes(t, tx))
	})
}

func BlockchainContains(t *testing.T, part *NodePartition, criteria func(txr *types.TransactionRecord) bool) func() bool {
	return func() bool {
		nodes := slices.Clone(part.Nodes)
		for len(nodes) > 0 {
			for ni, n := range nodes {
				number, err := n.LatestBlockNumber()
				if err != nil {
					t.Logf("partition node %s returned error: %v", n.peerConf.ID, err)
					continue
				}
				nodes[ni] = nil
				for i := uint64(0); i <= number; i++ {
					b, err := n.GetBlock(context.Background(), number-i)
					if err != nil || b == nil {
						continue
					}
					for _, t := range b.Transactions {
						if criteria(t) {
							return true
						}
					}
				}
			}
			nodes = slices.DeleteFunc(nodes, func(pn *partitionNode) bool { return pn == nil })
			time.Sleep(10 * time.Millisecond)
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

func createPeerConfs(count uint8) ([]*network.PeerConfiguration, peer.IDSlice, error) {
	var peerConfs = make([]*network.PeerConfiguration, count)

	// generate authentication keypairs
	keyPairs, err := generateKeyPairs(count)
	if err != nil {
		return nil, nil, err
	}

	var validators = make(peer.IDSlice, count)
	for i := 0; i < int(count); i++ {
		peerConfs[i], err = network.NewPeerConfiguration(
			"/ip4/127.0.0.1/tcp/0",
			nil,
			keyPairs[i], // authentication keypair. The ID of the node is derived from this keypair.
			nil,
		)
		if err != nil {
			return nil, nil, err
		}
		validators[i] = peerConfs[i].ID
	}
	sort.Sort(validators)

	return peerConfs, validators, nil
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
	}
	return keyPairs, nil
}
