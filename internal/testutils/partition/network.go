package testpartition

import (
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	"github.com/libp2p/go-libp2p-core/peerstore"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/forwarder"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txbuffer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	crypto2 "github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

// AlphabillNetwork for integration tests
type AlphabillNetwork struct {
	RootChain   *rootchain.RootChain
	Nodes       []*partition.Node
	ctxCancel   context.CancelFunc
	ctx         context.Context
	TxBuffers   []*txbuffer.TxBuffer
	BlockStores []store.BlockStore
}

// NewNetwork creates the AlphabillNetwork for integration tests. It starts partition nodes with given
// transaction system and a root chain.
func NewNetwork(partitionNodes int, txSystemProvider func() txsystem.TransactionSystem, systemIdentifier []byte) (*AlphabillNetwork, error) {
	if partitionNodes < 1 {
		return nil, errors.New("invalid count of partition Nodes")
	}
	// create network nodePeers
	nodePeers, err := createNetworkPeers(partitionNodes)
	if err != nil {
		return nil, err
	}

	// create signing keys
	signers, err := createSigners(partitionNodes)
	if err != nil {
		return nil, err
	}

	// create partition genesis file
	var nodeGenesisFiles = make([]*genesis.PartitionNode, partitionNodes)
	for i := 0; i < partitionNodes; i++ {
		nodeGenesis, err := partition.NewNodeGenesis(
			txSystemProvider(),
			partition.WithPeerID(nodePeers[i].ID()),
			partition.WithSigner(signers[i]),
			partition.WithSystemIdentifier(systemIdentifier),
		)
		if err != nil {
			return nil, err
		}
		nodeGenesisFiles[i] = nodeGenesis
	}

	// create partition genesis
	partitionRecord, err := partition.NewPartitionGenesis(nodeGenesisFiles, 2500)
	if err != nil {
		return nil, err
	}

	// create root genesis
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	rootGenesis, partitionGenesisFiles, err := rootchain.NewGenesis([]*genesis.PartitionRecord{partitionRecord}, rootSigner)
	if err != nil {
		return nil, err
	}

	// start root chain
	rootPeer, err := network.NewPeer(&network.PeerConfiguration{
		Address: "/ip4/127.0.0.1/tcp/0",
	})
	root, err := rootchain.NewRootChain(rootPeer, rootGenesis, rootSigner, rootchain.WithT3Timeout(900*time.Millisecond))

	partitionGenesis := partitionGenesisFiles[0]
	ctx, ctxCancel := context.WithCancel(context.Background())
	var txBuffers = make([]*txbuffer.TxBuffer, partitionNodes)
	var blockStores = make([]store.BlockStore, partitionNodes)
	// start Nodes
	var nodes = make([]*partition.Node, partitionNodes)
	for i := 0; i < partitionNodes; i++ {
		eventBus := eventbus.New()
		if err != nil {
			return nil, err
		}
		peer := nodePeers[i]
		peer.Network().Peerstore().AddAddrs(rootPeer.ID(), rootPeer.MultiAddresses(), peerstore.PermanentAddrTTL)
		buffer, err := txbuffer.New(1000, gocrypto.SHA256)
		if err != nil {
			return nil, err
		}
		txForwarder, err := forwarder.New(peer, 400*time.Millisecond, func(tx *transaction.Transaction) {
			buffer.Add(tx)
		})
		if err != nil {
			return nil, err
		}
		_, err = partition.NewLeaderSubscriber(peer.ID(), eventBus, buffer, txForwarder)
		if err != nil {
			return nil, err
		}
		_, err = partition.NewBlockCertificationSubscriber(peer, rootPeer.ID(), 10, eventBus)
		if err != nil {
			return nil, err
		}
		_, err = partition.NewBlockProposalSubscriber(peer, 10, 200*time.Millisecond, eventBus)
		if err != nil {
			return nil, err
		}
		blockStore := store.NewInMemoryBlockStore()
		n, err := partition.New(
			peer,
			signers[i],
			txSystemProvider(),
			partitionGenesis,
			partition.WithContext(ctx),
			partition.WithEventBus(eventBus),
			partition.WithBlockStore(blockStore),
		)
		if err != nil {
			return nil, err
		}
		nodes[i] = n
		txBuffers[i] = buffer
		blockStores[i] = blockStore
	}

	if err != nil {
		return nil, err
	}
	return &AlphabillNetwork{
		RootChain:   root,
		Nodes:       nodes,
		TxBuffers:   txBuffers,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		BlockStores: blockStores,
	}, nil
}

// BroadcastTx sends transactions to all nodes.
func (a *AlphabillNetwork) BroadcastTx(tx *transaction.Transaction) error {
	for _, buf := range a.TxBuffers {
		if err := buf.Add(tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (a *AlphabillNetwork) SubmitTx(tx *transaction.Transaction) error {
	return a.TxBuffers[0].Add(tx)
}

func (a *AlphabillNetwork) Close() error {
	a.ctxCancel()
	a.RootChain.Close()
	for _, node := range a.Nodes {
		node.Close()
	}
	return nil
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

func createNetworkPeers(count int) ([]*network.Peer, error) {
	var peers = make([]*network.Peer, count)
	// generate connection encryption key pairs
	keyPairs, err := generateKeyPairs(count)
	if err != nil {
		return nil, err
	}

	var peerInfo = make([]*network.PeerInfo, count)
	for i := 0; i < count; i++ {
		port, err := getFreePort()
		if err != nil {
			return nil, err
		}
		peerInfo[i] = &network.PeerInfo{
			Address:   fmt.Sprintf("/ip4/127.0.0.1/tcp/%v", port),
			PublicKey: keyPairs[i].PublicKey,
		}
	}

	// init Nodes
	for i := 0; i < count; i++ {
		p, err := network.NewPeer(&network.PeerConfiguration{
			Address:         peerInfo[i].Address,
			KeyPair:         keyPairs[i], // connection encryption key. The ID of the node is derived from this keypair.
			PersistentPeers: peerInfo,    // Persistent peers
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
		privateKey, publicKey, err := crypto2.GenerateEd25519Key(rand.Reader)
		privateKeyBytes, err := crypto2.MarshalPrivateKey(privateKey)
		if err != nil {
			return nil, err
		}
		pubKeyBytes, err := crypto2.MarshalPublicKey(publicKey)
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

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
