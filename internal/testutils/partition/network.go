package testpartition

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"google.golang.org/protobuf/proto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

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
	RootChain *rootchain.RootChain
	Nodes     []*partition.Node
	ctxCancel context.CancelFunc
	ctx       context.Context
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
			partition.WithSigningKey(signers[i]),
			partition.WithEncryptionPubKey(nodePeers[i].Configuration().KeyPair.PublicKey),
			partition.WithSystemIdentifier(systemIdentifier),
		)
		if err != nil {
			return nil, err
		}
		nodeGenesisFiles[i] = nodeGenesis
	}

	// create root genesis
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}

	rootPeer, err := network.NewPeer(&network.PeerConfiguration{
		Address: "/ip4/127.0.0.1/tcp/0",
	})

	encPubKey, err := rootPeer.PublicKey()
	if err != nil {
		return nil, err
	}
	pubKeyBytes, err := encPubKey.Raw()
	if err != nil {
		return nil, err
	}

	verifier, err := crypto.NewVerifierSecp256k1(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	rootGenesis, partitionGenesisFiles, err := rootchain.NewGenesisFromPartitionNodes(nodeGenesisFiles, 2500, rootSigner, verifier)
	if err != nil {
		return nil, err
	}

	// start root chain
	root, err := rootchain.NewRootChain(rootPeer, rootGenesis, rootSigner, rootchain.WithT3Timeout(900*time.Millisecond))

	partitionGenesis := partitionGenesisFiles[0]
	ctx, ctxCancel := context.WithCancel(context.Background())
	// start Nodes
	var nodes = make([]*partition.Node, partitionNodes)
	for i := 0; i < partitionNodes; i++ {
		if err != nil {
			return nil, err
		}
		peer := nodePeers[i]
		n, err := partition.New(
			peer,
			signers[i],
			txSystemProvider(),
			partitionGenesis,
			partition.WithContext(ctx),
			partition.WithDefaultEventProcessors(true),
			partition.WithRootAddressAndIdentifier(rootPeer.MultiAddresses()[0], rootPeer.ID()),
		)
		if err != nil {
			return nil, err
		}
		nodes[i] = n
	}

	if err != nil {
		return nil, err
	}
	return &AlphabillNetwork{
		RootChain: root,
		Nodes:     nodes,
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}, nil
}

// BroadcastTx sends transactions to all nodes.
func (a *AlphabillNetwork) BroadcastTx(tx *transaction.Transaction) error {
	for _, n := range a.Nodes {
		if err := n.SubmitTx(tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (a *AlphabillNetwork) SubmitTx(tx *transaction.Transaction) error {
	return a.Nodes[0].SubmitTx(tx)
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
		privateKey, publicKey, err := crypto2.GenerateSecp256k1Key(rand.Reader)
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

// BlockchainContainsTx checks if at least one partition node block contains the given transaction.
func BlockchainContainsTx(tx *transaction.Transaction, network *AlphabillNetwork) func() bool {
	return func() bool {
		for _, n := range network.Nodes {
			height := n.GetLatestBlock().GetBlockNumber()
			for i := uint64(0); i <= height; i++ {
				b, err := n.GetBlock(height - i)
				if err != nil || b == nil {
					continue
				}
				for _, t := range b.Transactions {
					if proto.Equal(t, tx) {
						return true
					}
				}
			}

		}
		return false
	}
}
