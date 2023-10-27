package testpartition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/validator/internal/network"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/partition"
	"github.com/alphabill-org/alphabill/validator/internal/rootchain"
	"github.com/alphabill-org/alphabill/validator/internal/rootchain/consensus/monolithic"
	rootgenesis "github.com/alphabill-org/alphabill/validator/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/rootchain/partitions"
	test "github.com/alphabill-org/alphabill/validator/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/validator/internal/testutils/net"
	testevent "github.com/alphabill-org/alphabill/validator/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/validator/pkg/logger"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

const rootValidatorNodes = 1

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	NodePartitions map[types.SystemID32]*NodePartition
	RootPartition  *RootPartition
	ctxCancel      context.CancelFunc
}

type RootPartition struct {
	rcGenesis *genesis.RootGenesis
	TrustBase map[string]crypto.Verifier
	Nodes     []*rootNode
	log       *slog.Logger
}

type NodePartition struct {
	systemId         types.SystemID
	partitionGenesis *genesis.PartitionGenesis
	txSystemFunc     func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem
	ctx              context.Context
	tb               map[string]crypto.Verifier
	Nodes            []*partitionNode
	log              *slog.Logger
}

type partitionNode struct {
	*partition.Node
	dbFile       string
	idxFile      string
	peerConf     *network.PeerConfiguration
	signer       crypto.Signer
	genesis      *genesis.PartitionNode
	EventHandler *testevent.TestEventHandler
	confOpts     []partition.NodeOption
	AddrGRPC     string
	cancel       context.CancelFunc
	done         chan error
}

type rootNode struct {
	*rootchain.Node
	EncKeyPair *network.PeerKeyPair
	RootSigner crypto.Signer
	genesis    *genesis.RootGenesis
	id         peer.ID
	addr       multiaddr.Multiaddr
	cancel     context.CancelFunc
	done       chan error
}

type ValidatorIndex int

const ANY_VALIDATOR ValidatorIndex = -1

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
func newRootPartition(nodePartitions []*NodePartition) (*RootPartition, error) {
	encKeyPairs, err := generateKeyPairs(rootValidatorNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption keypairs, %w", err)
	}
	rootSigners, err := createSigners(rootValidatorNodes)
	if err != nil {
		return nil, fmt.Errorf("create signer failed, %w", err)
	}
	trustBase := make(map[string]crypto.Verifier)
	rootNodes := make([]*rootNode, rootValidatorNodes)
	rootGenesisFiles := make([]*genesis.RootGenesis, rootValidatorNodes)
	for i := 0; i < rootValidatorNodes; i++ {
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
			rootgenesis.WithTotalNodes(rootValidatorNodes),
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
	port, err := net.GetFreePort()
	if err != nil {
		return fmt.Errorf("get free port failed, %w", err)
	}
	peerConf, err := network.NewPeerConfiguration(fmt.Sprintf("/ip4/127.0.0.1/tcp/%v", port), r.Nodes[0].EncKeyPair, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create peer configuration: %w", err)
	}

	rootPeer, err := network.NewPeer(ctx, peerConf, r.log)
	if err != nil {
		return fmt.Errorf("failed to create new peer node: %w", err)
	}
	log := r.log.With(logger.NodeID(rootPeer.ID()))
	rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, 300*time.Millisecond, log)
	if err != nil {
		return fmt.Errorf("failed to init root and partition nodes network, %w", err)
	}
	// Initiate partition store
	partitionStore, err := partitions.NewPartitionStoreFromGenesis(r.rcGenesis.Partitions)
	if err != nil {
		return fmt.Errorf("failed to create partition store form root genesis, %w", err)
	}
	cm, err := monolithic.NewMonolithicConsensusManager(rootPeer.ID().String(), r.rcGenesis, partitionStore, r.Nodes[0].RootSigner, log)
	if err != nil {
		return fmt.Errorf("consensus manager initialization failed, %w", err)
	}
	r.Nodes[0].Node, err = rootchain.New(rootPeer, rootNet, partitionStore, cm, log)
	if err != nil {
		return fmt.Errorf("failed to create root node, %w", err)
	}
	r.Nodes[0].addr = rootPeer.MultiAddresses()[0]
	r.Nodes[0].id = rootPeer.ID()
	// start root node
	rctx, rootCancel := context.WithCancel(ctx)
	r.Nodes[0].cancel = rootCancel
	r.Nodes[0].done = make(chan error, 1)
	go func(ec chan error) { ec <- r.Nodes[0].Run(rctx) }(r.Nodes[0].done)
	return nil
}

func NewPartition(t *testing.T, nodeCount int, txSystemProvider func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem, systemIdentifier []byte) (abPartition *NodePartition, err error) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			ctxCancel()
		}
	}()

	if nodeCount < 1 {
		return nil, fmt.Errorf("invalid count of partition Nodes: %d", nodeCount)
	}
	abPartition = &NodePartition{
		systemId:     systemIdentifier,
		txSystemFunc: txSystemProvider,
		Nodes:        make([]*partitionNode, nodeCount),
		log:          testlogger.New(t),
	}
	// create peer configurations
	peerConfs, err := createPeerConfs(ctx, nodeCount)
	if err != nil {
		return nil, err
	}
	// create partition signing keys
	signers, err := createSigners(nodeCount)
	if err != nil {
		return nil, err
	}
	for i := 0; i < nodeCount; i++ {
		peerConf := peerConfs[i]

		signer := signers[i]
		// create partition genesis file
		nodeGenesis, err := partition.NewNodeGenesis(
			txSystemProvider(map[string]crypto.Verifier{"genesis": nil}),
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

func (n *NodePartition) start(t *testing.T, ctx context.Context, rootID peer.ID, rootAddr multiaddr.Multiaddr) error {
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
		txIndexer, err := boltdb.New(nd.idxFile)
		if err != nil {
			return fmt.Errorf("unable to load tx indexer: %w", err)
		}
		t.Cleanup(func() { require.NoError(t, txIndexer.Close()) })
		nd.confOpts = append(nd.confOpts, partition.WithRootAddressAndIdentifier(rootAddr, rootID),
			partition.WithEventHandler(nd.EventHandler.HandleEvent, 100),
			partition.WithBlockStore(blockStore),
			partition.WithTxIndexer(txIndexer),
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
	log := n.log.With(logger.NodeID(pn.peerConf.ID))
	node, err := partition.NewNode(
		ctx,
		pn.peerConf,
		pn.signer,
		n.txSystemFunc(n.tb),
		n.partitionGenesis,
		nil,
		log,
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
	rootPartition, err := newRootPartition(nodePartitions)
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

func (a *AlphabillNetwork) Start(t *testing.T) error {
	a.RootPartition.log = testlogger.New(t)
	// create context
	ctx, ctxCancel := context.WithCancel(context.Background())
	if err := a.RootPartition.start(ctx); err != nil {
		ctxCancel()
		return fmt.Errorf("failed to start root partition, %w", err)
	}
	for id, part := range a.NodePartitions {
		// create one event handler per partition
		if err := part.start(t, ctx, a.RootPartition.Nodes[0].id, a.RootPartition.Nodes[0].addr); err != nil {
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

// WaitTxProof - uses the new validator index and endpoint and returns both transaction record and proof
// when tx has been executed and added to block
// todo: remove index when state proofs become available and refactor tests that require it to use unit proofs instead
func WaitTxProof(t *testing.T, part *NodePartition, idx ValidatorIndex, txOrder *types.TransactionOrder) (*types.TransactionRecord, *types.TxProof, error) {
	t.Helper()
	var (
		txRecord *types.TransactionRecord
		txProof  *types.TxProof
	)
	var nodes []*partitionNode
	if idx == ANY_VALIDATOR {
		nodes = part.Nodes
	} else {
		nodes = append(nodes, part.Nodes[idx])
	}
	txHash := txOrder.Hash(gocrypto.SHA256)
	if ok := eventually(func() bool {
		for _, n := range nodes {
			txRec, proof, err := n.GetTransactionRecord(context.Background(), txHash)
			if err != nil || proof == nil {
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

func createSigners(count int) ([]crypto.Signer, error) {
	var signers = make([]crypto.Signer, count)
	for i := 0; i < count; i++ {
		s, err := crypto.NewInMemorySecp256K1Signer()
		if err != nil {
			return nil, err
		}
		signers[i] = s
	}
	return signers, nil
}

func createPeerConfs(ctx context.Context, count int) ([]*network.PeerConfiguration, error) {
	var peerConfs = make([]*network.PeerConfiguration, count)

	// generate connection encryption key pairs
	keyPairs, err := generateKeyPairs(count)
	if err != nil {
		return nil, err
	}

	var validators = make(peer.IDSlice, count)

	for i := 0; i < count; i++ {
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

func generateKeyPairs(count int) ([]*network.PeerKeyPair, error) {
	var keyPairs = make([]*network.PeerKeyPair, count)
	for i := 0; i < count; i++ {
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
