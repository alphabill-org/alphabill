package testpartition

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"time"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/protobuf/proto"
)

// AlphabillPartition for integration tests
type AlphabillPartition struct {
	RootChain *rootchain.RootChain
	Nodes     []*partition.Node
	ctxCancel context.CancelFunc
	ctx       context.Context
}

// NewNetwork creates the AlphabillPartition for integration tests. It starts partition nodes with given
// transaction system and a root chain.
func NewNetwork(partitionNodes int, txSystemProvider func() txsystem.TransactionSystem, systemIdentifier []byte) (*AlphabillPartition, error) {
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
			partition.WithT2Timeout(2500),
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
	peerID, err := peer.IDFromPublicKey(encPubKey)
	if err != nil {
		return nil, err
	}
	pubKeyBytes, err := encPubKey.Raw()
	if err != nil {
		return nil, err
	}
	pr, err := rootchain.NewPartitionRecordFromNodes(nodeGenesisFiles)
	if err != nil {
		return nil, err
	}
	rootGenesis, partitionGenesisFiles, err := rootchain.NewRootGenesis(peerID.String(), rootSigner, pubKeyBytes, pr)
	if err != nil {
		return nil, err
	}

	// start root chain
	rootNet, err := network.NewLibP2PRootChainNetwork(rootPeer, 100, 300*time.Millisecond)
	if err != nil {
		return nil, err
	}
	root, err := rootchain.NewRootChain(rootPeer, rootGenesis, rootSigner, rootNet, rootchain.WithT3Timeout(900*time.Millisecond))

	partitionGenesis := partitionGenesisFiles[0]
	ctx, ctxCancel := context.WithCancel(context.Background())
	// start Nodes
	var nodes = make([]*partition.Node, partitionNodes)
	for i := 0; i < partitionNodes; i++ {
		if err != nil {
			return nil, err
		}
		peer := nodePeers[i]
		pn, err := network.NewLibP2PValidatorNetwork(peer, network.DefaultValidatorNetOptions)
		if err != nil {
			return nil, err
		}
		n, err := partition.New(
			peer,
			signers[i],
			txSystemProvider(),
			partitionGenesis,
			pn,
			partition.WithContext(ctx),
			partition.WithRootAddressAndIdentifier(rootPeer.MultiAddresses()[0], rootPeer.ID()),
		)
		if err != nil {
			return nil, err
		}
		nodes[i] = n
	}

	if err != nil {
		ctxCancel()
		return nil, err
	}
	return &AlphabillPartition{
		RootChain: root,
		Nodes:     nodes,
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}, nil
}

// BroadcastTx sends transactions to all nodes.
func (a *AlphabillPartition) BroadcastTx(tx *txsystem.Transaction) error {
	for _, n := range a.Nodes {
		if err := n.SubmitTx(tx); err != nil {
			return err
		}
	}
	return nil
}

// SubmitTx sends transactions to the first node.
func (a *AlphabillPartition) SubmitTx(tx *txsystem.Transaction) error {
	return a.Nodes[0].SubmitTx(tx)
}

func (a *AlphabillPartition) Close() error {
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
		port, err := net.GetFreePort()
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
		privateKey, publicKey, err := libp2pcrypto.GenerateSecp256k1Key(rand.Reader)
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

// BlockchainContainsTx checks if at least one partition node block contains the given transaction.
func BlockchainContainsTx(tx *txsystem.Transaction, network *AlphabillPartition) func() bool {
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
