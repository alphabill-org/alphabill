package partition

import (
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	rstore "github.com/alphabill-org/alphabill/internal/rootchain/store"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

type SingleNodePartition struct {
	nodeConf   *configuration
	store      store.BlockStore
	partition  *Node
	nodeDeps   *partitionStartupDependencies
	rootState  *rootchain.State
	rootSigner crypto.Signer
	mockNet    *testnetwork.MockNet
	eh         *testevent.TestEventHandler
}

type partitionStartupDependencies struct {
	peer        *network.Peer
	txSystem    txsystem.TransactionSystem
	nodeSigner  crypto.Signer
	genesis     *genesis.PartitionGenesis
	net         Net
	nodeOptions []NodeOption
}

func (t *AlwaysValidTransactionValidator) Validate(txsystem.GenericTransaction, uint64) error {
	return nil
}

func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

func NewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	p := createPeer(t)
	key, err := p.PublicKey()
	require.NoError(t, err)
	pubKeyBytes, err := key.Raw()
	require.NoError(t, err)

	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)

	systemId := []byte{1, 1, 1, 1}
	nodeGenesis, err := NewNodeGenesis(
		txSystem,
		WithPeerID("1"),
		WithSigningKey(nodeSigner),
		WithEncryptionPubKey(pubKeyBytes),
		WithSystemIdentifier(systemId),
		WithT2Timeout(2500),
	)
	require.NoError(t, err)

	// root genesis
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := rootchain.NewPartitionRecordFromNodes([]*genesis.PartitionNode{nodeGenesis})
	require.NoError(t, err)
	rootGenesis, partitionGenesis, err := rootchain.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
	if err != nil {
		t.Error(err)
	}
	require.NoError(t, err)

	// root chain
	rc, err := rootchain.NewState(rootGenesis, "test", rootSigner, rstore.NewInMemStateStore(gocrypto.SHA256))
	require.NoError(t, err)

	net := testnetwork.NewMockNetwork()
	eh := &testevent.TestEventHandler{}

	// allows restarting the node
	deps := &partitionStartupDependencies{
		peer:        p,
		txSystem:    txSystem,
		nodeSigner:  nodeSigner,
		genesis:     partitionGenesis[0],
		net:         net,
		nodeOptions: nodeOptions,
	}

	partition := &SingleNodePartition{
		nodeDeps:   deps,
		rootState:  rc,
		rootSigner: rootSigner,
		mockNet:    net,
		eh:         eh,
	}

	// partition node
	err = partition.StartNode()
	require.NoError(t, err)

	return partition
}

func (sn *SingleNodePartition) StartNode() error {
	n, err := New(
		sn.nodeDeps.peer,
		sn.nodeDeps.nodeSigner,
		sn.nodeDeps.txSystem,
		sn.nodeDeps.genesis,
		sn.nodeDeps.net,
		append([]NodeOption{
			WithT1Timeout(100 * time.Minute),
			WithLeaderSelector(&TestLeaderSelector{
				leader:      "1",
				currentNode: "1",
			}),
			WithTxValidator(&AlwaysValidTransactionValidator{}),
			WithEventHandler(sn.eh.HandleEvent, 100),
			WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		}, sn.nodeDeps.nodeOptions...)...,
	)
	if err != nil {
		return err
	}
	sn.partition = n
	sn.nodeConf = n.configuration
	sn.store = n.blockStore
	return nil
}

func (sn *SingleNodePartition) Restart(t *testing.T) {
	sn.partition.Close()
	fmt.Println("Restarting node...")
	require.NoError(t, sn.StartNode())
}

func (sn *SingleNodePartition) Close() {
	sn.partition.Close()
	close(sn.mockNet.MessageCh)
}

func (sn *SingleNodePartition) SubmitTx(tx *txsystem.Transaction) error {
	sn.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolInputForward,
		Message:  tx,
	})

	return nil
}

func (sn *SingleNodePartition) SubmitUnicityCertificate(uc *certificates.UnicityCertificate) {
	sn.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolUnicityCertificates,
		Message:  uc,
	})

}

func (sn *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) {
	sn.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolBlockProposal,
		Message:  prop,
	})
}

func (sn *SingleNodePartition) CreateUnicityCertificate(ir *certificates.InputRecord, roundNumber uint64, previousRoundRootHash []byte) (*certificates.UnicityCertificate, error) {
	id := sn.nodeConf.GetSystemIdentifier()
	sdrHash := sn.nodeConf.genesis.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	data := []*unicitytree.Data{{
		SystemIdentifier:            id,
		InputRecord:                 ir,
		SystemDescriptionRecordHash: sdrHash,
	},
	}
	ut, err := unicitytree.New(gocrypto.SHA256.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	unicitySeal, err := sn.createUnicitySeal(roundNumber, previousRoundRootHash, rootHash)
	if err != nil {
		return nil, err
	}
	cert, err := ut.GetCertificate(id)
	if err != nil {
		// this should never happen. if it does then exit with panic because we cannot generate
		// unicity tree certificates.
		panic(err)
	}

	return &certificates.UnicityCertificate{
		InputRecord: ir,
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier:      cert.SystemIdentifier,
			SiblingHashes:         cert.SiblingHashes,
			SystemDescriptionHash: sdrHash,
		},
		UnicitySeal: unicitySeal,
	}, nil
}

func (sn *SingleNodePartition) createUnicitySeal(roundNumber uint64, previousRoundRootHash, rootHash []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: roundNumber,
		PreviousHash:         previousRoundRootHash,
		Hash:                 rootHash,
	}
	return u, u.Sign("test", sn.rootSigner)
}

func (sn *SingleNodePartition) GetLatestBlock() *block.Block {
	return sn.store.LatestBlock()
}

func (sn *SingleNodePartition) CreateBlock(t *testing.T) {
	sn.SubmitT1Timeout(t)
	sn.SubmitUC(t, sn.IssueBlockUC(t))
}

func (sn *SingleNodePartition) SubmitUC(t *testing.T, uc *certificates.UnicityCertificate) {
	sn.eh.Reset()
	sn.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolUnicityCertificates,
		Message:  uc,
	})
	testevent.ContainsEvent(t, sn.eh, event.BlockFinalized)
}

func (sn *SingleNodePartition) IssueBlockUC(t *testing.T) *certificates.UnicityCertificate {
	req := sn.mockNet.SentMessages(network.ProtocolBlockCertification)[0].Message.(*certification.BlockCertificationRequest)
	sn.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	_, err := sn.rootState.HandleBlockCertificationRequest(req)
	require.NoError(t, err)
	newState, err := sn.rootState.CreateUnicityCertificates()
	require.NoError(t, err)
	uc, f := newState.Certificates[p.SystemIdentifier(req.SystemIdentifier)]
	if !f {
		t.Error("uc not created")
	}
	return uc
}

func (sn *SingleNodePartition) SubmitT1Timeout(t *testing.T) {
	sn.eh.Reset()
	sn.partition.timers.C <- &timer.Task{}
	require.Eventually(t, func() bool {
		return len(sn.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
}

type TestLeaderSelector struct {
	leader      peer.ID
	currentNode peer.ID
	mutex       sync.Mutex
}

func (l *TestLeaderSelector) SelfID() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.currentNode
}

// IsCurrentNodeLeader returns true it current node is the leader and must propose the next block.
func (l *TestLeaderSelector) IsCurrentNodeLeader() bool {
	return l.leader == l.SelfID()
}

func (l *TestLeaderSelector) UpdateLeader(seal *certificates.UnicitySeal) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if seal == nil {
		l.leader = ""
		return
	}
	l.leader = l.currentNode
	return
}

func (l *TestLeaderSelector) GetLeaderID() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

func (l *TestLeaderSelector) LeaderFromUnicitySeal(seal *certificates.UnicitySeal) peer.ID {
	if seal == nil {
		return ""
	}
	return l.currentNode
}

func createPeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{}
	// fake validator, so that network 'send' requests don't fail
	_, validatorPubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	validatorPubKeyBytes, _ := validatorPubKey.Raw()

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   "/ip4/1.2.3.4/tcp/80",
		PublicKey: validatorPubKeyBytes,
	}}
	//
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func NextBlockReceived(tp *SingleNodePartition, prevBlock *block.Block) func() bool {
	return func() bool {
		b := tp.GetLatestBlock()
		return b.UnicityCertificate.UnicitySeal.RootChainRoundNumber == prevBlock.UnicityCertificate.UnicitySeal.GetRootChainRoundNumber()+1
	}
}

func ContainsTransaction(block *block.Block, tx *txsystem.Transaction) bool {
	for _, t := range block.Transactions {
		if t == tx {
			return true
		}
	}
	return false
}

func CertificationRequestReceived(tp *SingleNodePartition) func() bool {
	return func() bool {
		messages := tp.mockNet.SentMessages(network.ProtocolBlockCertification)
		return len(messages) > 0
	}
}

func ContainsError(t *testing.T, tp *SingleNodePartition, errStr string) {
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.Error && strings.Contains(e.Content.(error).Error(), errStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
