package testpartition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/monolithic"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
)

const rootValidatorNodes = 1

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	NodePartitions map[p.SystemIdentifier]*NodePartition
	RootPartition  *RootPartition
	ctxCancel      context.CancelFunc
}

type RootPartition struct {
	rcGenesis *genesis.RootGenesis
	TrustBase map[string]crypto.Verifier
	Nodes     []*rootNode
}

type NodePartition struct {
	systemId         []byte
	partitionGenesis *genesis.PartitionGenesis
	txSystemFunc     func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem
	Nodes            []*partitionNode
}

type partitionNode struct {
	*partition.Node
	nodePeer     *network.Peer
	signer       crypto.Signer
	genesis      *genesis.PartitionNode
	EventHandler *testevent.TestEventHandler

	AddrGRPC string
	cancel   context.CancelFunc
	done     chan error
}

type rootNode struct {
	*rootchain.Node
	EncKeyPair *network.PeerKeyPair
	RootSigner crypto.Signer
	genesis    *genesis.RootGenesis
	id         peer.ID
	addr       multiaddr.Multiaddr
}

func (pn *partitionNode) Stop() error {
	if err := pn.nodePeer.Close(); err != nil {
		return err
	}

	pn.cancel()
	return <-pn.done
}

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
	rootPeer, err := network.NewPeer(ctx, &network.PeerConfiguration{
		Address: fmt.Sprintf("/ip4/127.0.0.1/tcp/%v", port),
		KeyPair: r.Nodes[0].EncKeyPair, // connection encryption key. The ID of the node is derived from this keypair.
	})

	if err != nil {
		return fmt.Errorf("failed to create new peer node: %w", err)
	}
	rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, 300*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to init root and partition nodes network, %w", err)
	}
	// Initiate partition store
	partitionStore, err := partitions.NewPartitionStoreFromGenesis(r.rcGenesis.Partitions)
	if err != nil {
		return fmt.Errorf("failed to create partition store form root genesis, %w", err)
	}
	cm, err := monolithic.NewMonolithicConsensusManager(rootPeer.ID().String(), r.rcGenesis, partitionStore, r.Nodes[0].RootSigner)
	if err != nil {
		return fmt.Errorf("consensus manager initialization failed, %w", err)
	}
	r.Nodes[0].Node, err = rootchain.New(rootPeer, rootNet, partitionStore, cm)
	if err != nil {
		return fmt.Errorf("failed to create root node, %w", err)
	}
	r.Nodes[0].addr = rootPeer.MultiAddresses()[0]
	r.Nodes[0].id = rootPeer.ID()
	// start root node
	go r.Nodes[0].Node.Run(ctx)
	return nil
}

func NewPartition(nodeCount int, txSystemProvider func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem, systemIdentifier []byte) (abPartition *NodePartition, err error) {
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
	}
	// create network nodePeers
	nodePeers, err := createNetworkPeers(ctx, nodeCount)
	if err != nil {
		return nil, err
	}
	// create partition signing keys
	signers, err := createSigners(nodeCount)
	if err != nil {
		return nil, err
	}
	for i := 0; i < nodeCount; i++ {
		peer := nodePeers[i]
		signer := signers[i]
		// create partition genesis file
		nodeGenesis, err := partition.NewNodeGenesis(
			txSystemProvider(map[string]crypto.Verifier{"genesis": nil}),
			partition.WithPeerID(peer.ID()),
			partition.WithSigningKey(signer),
			partition.WithEncryptionPubKey(peer.Configuration().KeyPair.PublicKey),
			partition.WithSystemIdentifier(systemIdentifier),
			partition.WithT2Timeout(2500),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create node genesis, %w", err)
		}
		abPartition.Nodes[i] = &partitionNode{
			genesis:  nodeGenesis,
			nodePeer: peer,
			signer:   signer,
		}
	}
	return abPartition, nil
}

func (n *NodePartition) start(ctx context.Context, rootID peer.ID, rootAddr multiaddr.Multiaddr) error {
	// start Nodes
	trustBase, err := genesis.NewValidatorTrustBase(n.partitionGenesis.RootValidators)
	if err != nil {
		return fmt.Errorf("failed to extract root trust base from genesis file, %w", err)
	}

	for _, nd := range n.Nodes {
		pn, err := network.NewLibP2PValidatorNetwork(nd.nodePeer, network.DefaultValidatorNetOptions)
		if err != nil {
			return err
		}
		nd.EventHandler = &testevent.TestEventHandler{}
		node, err := partition.New(
			nd.nodePeer,
			nd.signer,
			n.txSystemFunc(trustBase),
			n.partitionGenesis,
			pn,
			partition.WithRootAddressAndIdentifier(rootAddr, rootID),
			partition.WithEventHandler(nd.EventHandler.HandleEvent, 100),
		)
		if err != nil {
			return fmt.Errorf("failed to start partition, %w", err)
		}

		nctx, ncfn := context.WithCancel(ctx)
		nd.Node = node
		nd.cancel = ncfn
		nd.done = make(chan error, 1)
		go func(ec chan error) { ec <- node.Run(nctx) }(nd.done)
	}

	for _, nd := range n.Nodes {
		if err = assertConnections(nd.nodePeer, len(n.Nodes)); err != nil {
			return err
		}
	}
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
	nodeParts := make(map[p.SystemIdentifier]*NodePartition)
	for _, part := range nodePartitions {
		nodeParts[p.SystemIdentifier(part.systemId)] = part
	}
	return &AlphabillNetwork{
		RootPartition:  rootPartition,
		NodePartitions: nodeParts,
	}, nil
}

func (a *AlphabillNetwork) Start() error {
	// create context
	ctx, ctxCancel := context.WithCancel(context.Background())
	if err := a.RootPartition.start(ctx); err != nil {
		ctxCancel()
		return fmt.Errorf("failed to start root partition, %w", err)
	}
	for id, part := range a.NodePartitions {
		// create one event handler per partition
		if err := part.start(ctx, a.RootPartition.Nodes[0].id, a.RootPartition.Nodes[0].addr); err != nil {
			ctxCancel()
			return fmt.Errorf("failed to start partition %X, %w", id.Bytes(), err)
		}
	}
	a.ctxCancel = ctxCancel
	return nil
}

func (a *AlphabillNetwork) Close() error {
	a.ctxCancel()
	return nil
}

func (a *AlphabillNetwork) GetNodePartition(sysID []byte) (*NodePartition, error) {
	part, f := a.NodePartitions[p.SystemIdentifier(sysID)]
	if !f {
		return nil, fmt.Errorf("unknown partition %X", sysID)
	}
	return part, nil
}

// BroadcastTx sends transactions to all nodes.
func (n *NodePartition) BroadcastTx(tx *txsystem.Transaction) error {
	for _, n := range n.Nodes {
		if err := n.SubmitTx(context.Background(), tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (n *NodePartition) SubmitTx(tx *txsystem.Transaction) error {
	return n.Nodes[0].SubmitTx(context.Background(), tx)
}

type TxConverter func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)

func (c TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return c(tx)
}

func (n *NodePartition) GetBlockProof(tx *txsystem.Transaction, txConverter TxConverter) (*block.GenericBlock, *block.BlockProof, error) {
	for _, n := range n.Nodes {
		bl, err := n.GetLatestBlock()
		if err != nil {
			return nil, nil, err
		}
		number := bl.UnicityCertificate.InputRecord.RoundNumber
		for i := uint64(0); i < number; i++ {
			b, err := n.GetBlock(context.Background(), number-i)
			if err != nil || b == nil {
				continue
			}
			for _, t := range b.Transactions {
				if bytes.Equal(t.TxBytes(), tx.TxBytes()) {
					genBlock, err := b.ToGenericBlock(txConverter)
					if err != nil {
						return nil, nil, err
					}
					proof, err := block.NewPrimaryProof(genBlock, tx.UnitId, gocrypto.SHA256)
					if err != nil {
						return nil, nil, err
					}
					return genBlock, proof, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("tx with id %x was not found", tx.UnitId)
}

// BlockchainContainsTx checks if at least one partition node block contains the given transaction.
func BlockchainContainsTx(part *NodePartition, tx *txsystem.Transaction) func() bool {
	return BlockchainContains(part, func(actualTx *txsystem.Transaction) bool {
		// compare tx without server metadata field
		return bytes.Equal(tx.TxBytes(), actualTx.TxBytes()) && proto.Equal(tx.TransactionAttributes, actualTx.TransactionAttributes)
	})
}

func BlockchainContains(part *NodePartition, criteria func(tx *txsystem.Transaction) bool) func() bool {
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
					if criteria(t) {
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

func createNetworkPeers(ctx context.Context, count int) ([]*network.Peer, error) {
	var peers = make([]*network.Peer, count)
	// generate connection encryption key pairs
	keyPairs, err := generateKeyPairs(count)
	if err != nil {
		return nil, err
	}

	var validators = make(peer.IDSlice, count)

	for i := 0; i < count; i++ {
		nodeID, err := network.NodeIDFromPublicKeyBytes(keyPairs[i].PublicKey)
		if err != nil {
			return nil, err
		}

		validators[i] = nodeID
	}
	sort.Sort(validators)

	// init Nodes
	for i := 0; i < count; i++ {
		p, err := network.NewPeer(ctx, &network.PeerConfiguration{
			Address:    "/ip4/127.0.0.1/tcp/0",
			KeyPair:    keyPairs[i], // connection encryption key. The ID of the node is derived from this keypair.
			Validators: validators,  // Persistent peers
		})

		if err != nil {
			return nil, err
		}
		peers[i] = p
	}

	return peers, nil
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

func assertConnections(p *network.Peer, count int) error {
	ch := make(chan bool, 1)

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			return errors.New("network not initialized")
		case <-tick:
			tick = nil
			go func() {
				ch <- func() bool {
					return p.Network().Peerstore().Peers().Len() >= count
				}()
			}()
		case v := <-ch:
			if v {
				return nil
			}
			tick = ticker.C
		}
	}
}
