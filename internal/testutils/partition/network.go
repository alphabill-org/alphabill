package testpartition

import (
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/distributed"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/protobuf/proto"
)

// AlphabillPartition for integration tests
type AlphabillPartition struct {
	RootNodes    []*rootNode
	Nodes        []*partitionNode
	ctxCancel    context.CancelFunc
	TrustBase    map[string]crypto.Verifier
	EventHandler *testevent.TestEventHandler
}

const rootValidatorNodes = 3

type partitionNode struct {
	*partition.Node
	cancel context.CancelFunc
	done   chan error
}

type rootNode struct {
	*rootchain.Node
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

// NewNetwork creates the AlphabillPartition for integration tests. It starts partition nodes with given
// transaction system and a root chain.
func NewNetwork(nodeCount int, txSystemProvider func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem, systemIdentifier []byte) (*AlphabillPartition, error) {
	if nodeCount < 1 {
		return nil, fmt.Errorf("invalid count of partition Nodes: %d", nodeCount)
	}
	// create network nodePeers
	nodePeers, err := createNetworkPeers(nodeCount)
	if err != nil {
		return nil, err
	}
	// create partition signing keys
	signers, err := createSigners(nodeCount)
	if err != nil {
		return nil, err
	}
	var transactionSystems []txsystem.TransactionSystem
	// create root nodes and signers keys
	rootPeers, err := createNetworkPeers(rootValidatorNodes)
	if err != nil {
		return nil, err
	}
	rootSigners, err := createSigners(rootValidatorNodes)
	if err != nil {
		return nil, err
	}

	// set-up trust base
	trustBase := make(map[string]crypto.Verifier)
	for i := 0; i < rootValidatorNodes; i++ {
		trustBase[rootPeers[i].ID().String()], err = rootSigners[i].Verifier()
		if err != nil {
			return nil, err
		}
	}
	// create partition genesis file
	var nodeGenesisFiles = make([]*genesis.PartitionNode, nodeCount)
	for i := 0; i < nodeCount; i++ {
		transactionSystem := txSystemProvider(trustBase)
		nodeGenesis, err := partition.NewNodeGenesis(
			transactionSystem,
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
		transactionSystems = append(transactionSystems, transactionSystem)
		transactionSystem.Revert()
	}

	// create root genesis
	pr, err := rootgenesis.NewPartitionRecordFromNodes(nodeGenesisFiles)
	if err != nil {
		return nil, err
	}
	// create root validator genesis files
	rootGenesisFiles := make([]*genesis.RootGenesis, rootValidatorNodes)
	for i := 0; i < rootValidatorNodes; i++ {
		encPubKey, err := rootPeers[i].PublicKey()
		if err != nil {
			return nil, err
		}
		pubKeyBytes, err := encPubKey.Raw()
		if err != nil {
			return nil, err
		}
		rg, _, err := rootgenesis.NewRootGenesis(
			rootPeers[i].ID().String(),
			rootSigners[i],
			pubKeyBytes,
			pr,
			rootgenesis.WithTotalNodes(rootValidatorNodes),
			rootgenesis.WithBlockRate(genesis.MinBlockRateMs),
			rootgenesis.WithConsensusTimeout(genesis.DefaultConsensusTimeout))
		if err != nil {
			return nil, err
		}
		rootGenesisFiles[i] = rg
	}
	rootGenesis, partitionGenesisFiles, err := rootgenesis.MergeRootGenesisFiles(rootGenesisFiles)
	if err != nil {
		return nil, err
	}
	ctx, ctxCancel := context.WithCancel(context.Background())
	// start root chain nodes
	rootNodes := make([]*rootNode, rootValidatorNodes)
	rootPartitionHots := make([]*network.Peer, rootValidatorNodes)
	for i := 0; i < rootValidatorNodes; i++ {
		partitionHost, err := network.NewPeer(&network.PeerConfiguration{
			Address: "/ip4/127.0.0.1/tcp/0",
		})
		rootPartitionHots[i] = partitionHost
		rootNet, err := network.NewLibP2PRootChainNetwork(partitionHost, 100, 300*time.Millisecond)
		if err != nil {
			ctxCancel()
			return nil, err
		}
		rootConsensusNet, err := network.NewLibP2RootConsensusNetwork(rootPeers[i], 100, 300*time.Millisecond)
		// Initiate partition store
		partitionStore, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
		if err != nil {
			ctxCancel()
			return nil, fmt.Errorf("failed to extract partition info from genesis, %w", err)
		}
		// Create distributed consensus manager
		cm, err := distributed.NewDistributedAbConsensusManager(rootPeers[i], rootGenesis, partitionStore, rootConsensusNet, rootSigners[i])
		if err != nil {
			ctxCancel()
			return nil, fmt.Errorf("consensus manager initialization failed, %w", err)
		}
		rn, err := rootchain.New(partitionHost, rootNet, partitionStore, cm)
		if err != nil {
			ctxCancel()
			return nil, err
		}
		nctx, ncfn := context.WithCancel(ctx)
		rootNodes[i] = &rootNode{Node: rn, cancel: ncfn, done: make(chan error, 1)}
		// start root node
		go func(ec chan error) { ec <- rn.Start(nctx) }(rootNodes[i].done)
	}
	partitionGenesis := partitionGenesisFiles[0]
	// start Nodes
	nodes := make([]*partitionNode, nodeCount)
	eh := &testevent.TestEventHandler{}
	for i := 0; i < nodeCount; i++ {
		if err != nil {
			ctxCancel()
			return nil, err
		}
		peer := nodePeers[i]
		pn, err := network.NewLibP2PValidatorNetwork(peer, network.DefaultValidatorNetOptions)
		if err != nil {
			ctxCancel()
			return nil, err
		}
		n, err := partition.New(
			peer,
			signers[i],
			transactionSystems[i],
			partitionGenesis,
			pn,
			partition.WithRootAddressAndIdentifier(rootPartitionHots[0].MultiAddresses()[0], rootPartitionHots[0].ID()),
			partition.WithEventHandler(eh.HandleEvent, 100),
		)
		if err != nil {
			ctxCancel()
			return nil, err
		}

		nctx, ncfn := context.WithCancel(ctx)
		nodes[i] = &partitionNode{Node: n, cancel: ncfn, done: make(chan error, 1)}
		go func(ec chan error) { ec <- n.Run(nctx) }(nodes[i].done)
	}

	if err != nil {
		ctxCancel()
		return nil, err
	}
	return &AlphabillPartition{
		RootNodes:    rootNodes,
		Nodes:        nodes,
		ctxCancel:    ctxCancel,
		TrustBase:    trustBase,
		EventHandler: eh,
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

type TxConverter func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)

func (c TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return c(tx)
}

func (a *AlphabillPartition) GetBlockProof(tx *txsystem.Transaction, txConverter TxConverter) (*block.GenericBlock, *block.BlockProof, error) {
	for _, n := range a.Nodes {
		bl, err := n.GetLatestBlock()
		if err != nil {
			return nil, nil, err
		}
		number := bl.UnicityCertificate.InputRecord.RoundNumber
		for i := uint64(0); i < number; i++ {
			b, err := n.GetBlock(number - i)
			if err != nil || b == nil {
				continue
			}
			for _, t := range b.Transactions {
				if proto.Equal(t, tx) {
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

func (a *AlphabillPartition) Close() error {
	a.ctxCancel()
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

// BlockchainContainsTx checks if at least one partition node block contains the given transaction.
func BlockchainContainsTx(tx *txsystem.Transaction, network *AlphabillPartition) func() bool {
	return BlockchainContains(network, func(t *txsystem.Transaction) bool {
		return proto.Equal(t, tx)
	})
}

func BlockchainContains(network *AlphabillPartition, criteria func(tx *txsystem.Transaction) bool) func() bool {
	return func() bool {
		for _, n := range network.Nodes {
			bl, err := n.GetLatestBlock()
			if err != nil {
				panic(err)
			}
			number := bl.UnicityCertificate.InputRecord.RoundNumber
			for i := uint64(0); i <= number; i++ {
				b, err := n.GetBlock(number - i)
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
