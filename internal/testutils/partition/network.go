package testpartition

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"testing"
	"time"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rootchain"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/alphabill-org/alphabill/txsystem"
)

const networkID = 5
const speedFactor = 4

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	RootChain *RootChain
	Shards    map[types.PartitionShardID]*Shard
	ctx       context.Context
	ctxCancel context.CancelFunc
}

type RootChain struct {
	TrustBase types.RootTrustBase
	nodes     []*rootNode
}

type Shard struct {
	shardConf *types.PartitionDescriptionRecord
	Nodes     []*shardNode
	ctx       context.Context
	ctxCancel context.CancelFunc
}

type shardNode struct {
	*partition.Node
	nodeConf     *partition.NodeConf
	EventHandler *testevent.TestEventHandler
	txSystem     txsystem.TransactionSystem

	ctxCancel context.CancelFunc
	done      chan error
}

type rootNode struct {
	*rootchain.Node
	RootSigner       abcrypto.Signer
	peerConf         *network.PeerConfiguration
	consensusManager *consensus.ConsensusManager
	orchestration    *partitions.Orchestration
	addr             []multiaddr.Multiaddr
	homeDir          string

	ctxCancel context.CancelFunc
	done      chan error
}

type txSystemProvider func(trustBase types.RootTrustBase) txsystem.TransactionSystem

func (n *shardNode) Stop() error {
	n.ctxCancel()
	return <-n.done
}

func (n *rootNode) Stop() error {
	n.ctxCancel()
	return <-n.done
}

const testNetworkTimeout = 600 * time.Millisecond

func NewAlphabillNetwork(t *testing.T, rootNodeCount int) *AlphabillNetwork {
	nodes, nodeInfos := testutils.CreateTestNodes(t, rootNodeCount)
	rootNodes := make([]*rootNode, rootNodeCount)

	for i, node := range nodes {
		rootNodes[i] = &rootNode{
			RootSigner: node.Signer,
			peerConf:   node.PeerConf,
			homeDir:    t.TempDir(),
		}
	}
	trustBase, err := types.NewTrustBaseGenesis(networkID, nodeInfos)
	require.NoError(t, err)

	return &AlphabillNetwork{
		RootChain: &RootChain{
			TrustBase: trustBase,
			nodes:     rootNodes,
		},
		Shards: make(map[types.PartitionShardID]*Shard),
	}
}

// Adds a shard to a running AlphabillNetwork instance
func (a *AlphabillNetwork) AddShard(t *testing.T, shardConf *types.PartitionDescriptionRecord, nodeCount int, txSystemProvider txSystemProvider) {
	require.NotZero(t, len(a.RootChain.nodes), "AlphabillNetwork must contain at least one root node")
	require.NotNil(t, a.RootChain.nodes[0].addr, "AlphabillNetwork must be running to add shards")

	nodes, nodeInfos := testutils.CreateTestNodes(t, nodeCount)
	shardConf.Validators = nodeInfos

	// Add new shard to root chain
	for _, n := range a.RootChain.nodes {
		require.NoError(t, n.orchestration.AddShardConfig(shardConf))
	}

	// Run shard nodes only after root chain has activated the shard,
	// then we won't have delays because of failed handshakes
	a.waitShardActivation(t, shardConf.PartitionID, shardConf.ShardID)

	shard := &Shard{
		shardConf: shardConf,
		Nodes:     make([]*shardNode, nodeCount),
	}

	for idx, node := range nodes {
		ctx, ctxCancel := context.WithCancel(a.ctx)
		eventHandler := &testevent.TestEventHandler{}
		log := testlogger.New(t).With(logger.NodeID(node.PeerConf.ID))
		obs := observability.WithLogger(testobserve.Default(t), log)

		bootNode := a.RootChain.nodes[0]
		bootstrapAddress := fmt.Sprintf("%s/p2p/%s", bootNode.addr[0], bootNode.peerConf.ID)

		nodeConf, err := partition.NewNodeConf(
			node.KeyConf(t),
			shardConf,
			a.RootChain.TrustBase,
			obs,
			partition.WithAddress("/ip4/127.0.0.1/tcp/0"),
			partition.WithBootstrapAddresses([]string{bootstrapAddress}),
			partition.WithEventHandler(eventHandler.HandleEvent, 100),
			partition.WithT1Timeout(partition.DefaultT1Timeout/speedFactor),
			partition.WithOwnerIndex(partition.NewOwnerIndexer(obs.Logger())),
		)
		require.NoError(t, err)

		txSystem := txSystemProvider(a.RootChain.TrustBase)
		node, err := partition.NewNode(
			ctx,
			txSystem,
			nodeConf,
		)
		require.NoError(t, err)

		shard.Nodes[idx] = &shardNode{
			Node:         node,
			nodeConf:     nodeConf,
			txSystem:     txSystem,
			EventHandler: eventHandler,
			ctxCancel:    ctxCancel,
		}

		shard.Nodes[idx].done = make(chan error, 1)
		go func(ec chan error) {
			ec <- node.Run(ctx)
		}(shard.Nodes[idx].done)
	}

	a.Shards[shard.PartitionShardID()] = shard

	// make sure node network (to other nodes and root nodes) is initiated
	for _, nd := range shard.Nodes {
		require.Eventually(t, func() bool {
			return len(nd.Peer().Network().Peers()) >= len(shard.Nodes)
		}, 2*time.Second, 100*time.Millisecond)
	}
}

func (a *AlphabillNetwork) waitShardActivation(t *testing.T, partitionID types.PartitionID, shardID types.ShardID) {
	require.NotEmpty(t, a.RootChain.nodes)
	require.Eventually(t, func() bool {
		cm := a.RootChain.nodes[0].consensusManager
		_, err := cm.ShardInfo(partitionID, shardID)
		return err == nil
	}, test.WaitDuration, test.WaitTick)
}

// Start AB network, no bootstrap all id's and addresses are injected to peer store at start
func (a *AlphabillNetwork) Start(t *testing.T) error {
	a.ctx, a.ctxCancel = context.WithCancel(context.Background())
	require.NotEmpty(t, a.RootChain.nodes)

	if err := a.RootChain.start(t, a.ctx); err != nil {
		a.ctxCancel()
		return fmt.Errorf("failed to start root chain, %w", err)
	}
	return nil
}

func (a *AlphabillNetwork) Close() (retErr error) {
	a.ctxCancel()
	// wait and check validator exit
	for _, shard := range a.Shards {
		// stop all nodes
		for _, n := range shard.Nodes {
			if err := n.Peer().Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("peer close error: %w", err))
			}
			nodeErr := <-n.done
			if !errors.Is(nodeErr, context.Canceled) {
				retErr = errors.Join(retErr, nodeErr)
			}
		}
	}

	// check root exit
	for _, rNode := range a.RootChain.nodes {
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

func (a *AlphabillNetwork) GetShard(psID types.PartitionShardID) (*Shard, error) {
	shard, f := a.Shards[psID]
	if !f {
		return nil, fmt.Errorf("unknown shard %s", psID)
	}
	return shard, nil
}

func (a *AlphabillNetwork) GetValidator(psID types.PartitionShardID) (partition.UnicityCertificateValidator, error) {
	shard, f := a.Shards[psID]
	if !f {
		return nil, fmt.Errorf("unknown shard %s", psID)
	}
	return partition.NewDefaultUnicityCertificateValidator(shard.shardConf, a.RootChain.TrustBase, crypto.SHA256)
}

func (r *RootChain) start(t *testing.T, ctx context.Context) error {
	// start root nodes
	var bootNode []peer.AddrInfo
	for _, rn := range r.nodes {
		// root node context
		ctx, rn.ctxCancel = context.WithCancel(ctx)

		log := testlogger.New(t).With(logger.NodeID(rn.peerConf.ID))
		obs := observability.WithLogger(testobserve.Default(t), log)

		if bootNode != nil {
			rn.peerConf.BootstrapPeers = bootNode
		}
		rootPeer, err := network.NewPeer(ctx, rn.peerConf, log, nil)
		if err != nil {
			return fmt.Errorf("failed to create root peer node: %w", err)
		}
		if bootNode != nil {
			if err = rootPeer.BootstrapConnect(ctx, log); err != nil {
				return fmt.Errorf("root node bootstrap failed, %w", err)
			}
		} else {
			bootNode = []peer.AddrInfo{{
				ID:    rootPeer.ID(),
				Addrs: rootPeer.MultiAddresses(),
			}}
		}

		rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init root and partition nodes network, %w", err)
		}

		rootConsensusNet, err := network.NewLibP2RootConsensusNetwork(rootPeer, 100, testNetworkTimeout, obs)
		if err != nil {
			return fmt.Errorf("failed to init consensus network, %w", err)
		}

		orchestration, err := partitions.NewOrchestration(networkID, filepath.Join(rn.homeDir, "orchestration.db"), testlogger.New(t))
		if err != nil {
			return fmt.Errorf("failed to init orchestration: %w", err)
		}

		consensusParams := consensus.NewConsensusParams()
		consensusParams.BlockRate /= speedFactor

		cm, err := consensus.NewConsensusManager(
			rootPeer.ID(),
			r.TrustBase,
			orchestration,
			rootConsensusNet,
			rn.RootSigner,
			obs,
			consensus.WithConsensusParams(*consensusParams))
		if err != nil {
			return fmt.Errorf("consensus manager initialization failed, %w", err)
		}
		node, err := rootchain.New(rootPeer, rootNet, cm, obs)
		if err != nil {
			return fmt.Errorf("failed to create root node, %w", err)
		}
		rn.Node = node
		rn.consensusManager = cm
		rn.orchestration = orchestration
		rn.addr = rootPeer.MultiAddresses()

		// start root node
		rn.done = make(chan error, 1)
		go func(ctx context.Context, ec chan error) {
			ec <- node.Run(ctx)
		}(ctx, rn.done)
	}
	return nil
}

func (s *Shard) PartitionShardID() types.PartitionShardID {
	return types.PartitionShardID{
		PartitionID: s.shardConf.PartitionID,
		ShardID:     s.shardConf.ShardID.Key(),
	}
}

// BroadcastTx sends transactions to all nodes.
func (n *Shard) BroadcastTx(tx *types.TransactionOrder) error {
	for _, n := range n.Nodes {
		if _, err := n.SubmitTx(context.Background(), tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (n *Shard) SubmitTx(tx *types.TransactionOrder) error {
	_, err := n.Nodes[0].SubmitTx(context.Background(), tx)
	return err
}

func (n *Shard) GetTxProof(t *testing.T, tx *types.TransactionOrder) (*types.Block, *types.TxRecordProof, error) {
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

// ShardInitReady - all nodes are in normal state and return the latest block number
func ShardInitReady(t *testing.T, shard *Shard) func() bool {
	t.Helper()
	return func() bool {
		for _, n := range shard.Nodes {
			_, err := n.LatestBlockNumber()
			if err != nil {
				return false
			}
		}
		return true
	}
}

// WaitTxProof - wait for proof from any validator in a shard. If one has the proof it does not mean all have processed
// the UC. Returns both transaction record and proof when tx has been executed and added to block
func WaitTxProof(t *testing.T, shard *Shard, txOrder *types.TransactionOrder) (*types.TxRecordProof, error) {
	t.Helper()
	var txRecordProof *types.TxRecordProof
	txHash := test.DoHash(t, txOrder)

	require.Eventually(t, func() bool {
		for _, n := range shard.Nodes {
			txRecProof, err := n.GetTransactionRecordProof(context.Background(), txHash)
			if errors.Is(err, partition.ErrIndexNotFound) {
				continue
			}
			txRecordProof = txRecProof
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)

	return txRecordProof, nil
}

func WaitUnitProof(t *testing.T, shard *Shard, ID types.UnitID, txOrder *types.TransactionOrder) (*types.UnitStateWithProof, error) {
	t.Helper()
	var unitProof *types.UnitStateWithProof
	txHash := test.DoHash(t, txOrder)

	require.Eventually(t, func() bool {
		for _, n := range shard.Nodes {
			unitDataAndProof, err := partition.ReadUnitProofIndex(n.nodeConf.ProofStore(), ID, txHash)
			if err != nil {
				continue
			}
			unitProof = unitDataAndProof
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)

	return unitProof, nil
}

// BlockchainContainsTx checks if at least one shard node block contains the given transaction.
func BlockchainContainsTx(t *testing.T, shard *Shard, tx *types.TransactionOrder) func() bool {
	return BlockchainContains(t, shard, func(actualTx *types.TransactionRecord) bool {
		return bytes.Equal(actualTx.TransactionOrder, testtransaction.TxoToBytes(t, tx))
	})
}

// BlockchainContainsSuccessfulTx checks if at least one shard node has successfully executed the given transaction.
func BlockchainContainsSuccessfulTx(t *testing.T, shard *Shard, tx *types.TransactionOrder) func() bool {
	return BlockchainContains(t, shard, func(actualTx *types.TransactionRecord) bool {
		return actualTx.ServerMetadata.SuccessIndicator == types.TxStatusSuccessful &&
			bytes.Equal(actualTx.TransactionOrder, testtransaction.TxoToBytes(t, tx))
	})
}

func BlockchainContains(t *testing.T, shard *Shard, criteria func(txr *types.TransactionRecord) bool) func() bool {
	return func() bool {
		nodes := slices.Clone(shard.Nodes)
		for len(nodes) > 0 {
			for ni, n := range nodes {
				number, err := n.LatestBlockNumber()
				if err != nil {
					t.Logf("shard node %s returned error: %v", n.Peer().ID(), err)
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
			nodes = slices.DeleteFunc(nodes, func(pn *shardNode) bool { return pn == nil })
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
}
