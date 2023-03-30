package partition

import (
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
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
	store      keyvaluedb.KeyValueDB
	partition  *Node
	nodeDeps   *partitionStartupDependencies
	rootRound  uint64
	certs      map[p.SystemIdentifier]*certificates.UnicityCertificate
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

func SetupNewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	peer := createPeer(t)
	key, err := peer.PublicKey()
	require.NoError(t, err)
	pubKeyBytes, err := key.Raw()
	require.NoError(t, err)

	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)
	systemId := []byte{1, 1, 1, 1}
	nodeGenesis, err := NewNodeGenesis(
		txSystem,
		WithPeerID(peer.ID()),
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
	pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{nodeGenesis})
	require.NoError(t, err)
	rootGenesis, partitionGenesis, err := rootgenesis.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
	if err != nil {
		t.Error(err)
	}
	require.NoError(t, err)

	// root state
	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		certs[partition.GetSystemIdentifierString()] = partition.Certificate
	}

	net := testnetwork.NewMockNetwork()
	eh := &testevent.TestEventHandler{}

	// allows restarting the node
	deps := &partitionStartupDependencies{
		peer:        peer,
		txSystem:    txSystem,
		nodeSigner:  nodeSigner,
		genesis:     partitionGenesis[0],
		net:         net,
		nodeOptions: nodeOptions,
	}

	partition := &SingleNodePartition{
		nodeDeps:   deps,
		rootRound:  rootGenesis.GetRoundNumber(),
		certs:      certs,
		rootSigner: rootSigner,
		mockNet:    net,
		eh:         eh,
	}
	return partition
}

func StartSingleNodePartition(t *testing.T, p *SingleNodePartition) {
	// partition node
	require.NoError(t, p.StartNode())
}

func NewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	partition := SetupNewSingleNodePartition(t, txSystem, nodeOptions...)
	StartSingleNodePartition(t, partition)
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
				leader:      sn.nodeDeps.peer.ID(),
				currentNode: sn.nodeDeps.peer.ID(),
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

func (sn *SingleNodePartition) SubmitTxFromRPC(tx *txsystem.Transaction) error {
	return sn.partition.SubmitTx(tx)
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

func (sn *SingleNodePartition) CreateUnicityCertificate(ir *certificates.InputRecord, roundNumber uint64) (*certificates.UnicityCertificate, error) {
	sdr := sn.nodeDeps.genesis.SystemDescriptionRecord
	sdrHash := sdr.Hash(gocrypto.SHA256)
	data := []*unicitytree.Data{{
		SystemIdentifier:            sdr.SystemIdentifier,
		InputRecord:                 ir,
		SystemDescriptionRecordHash: sdrHash,
	},
	}
	ut, err := unicitytree.New(gocrypto.SHA256.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	unicitySeal, err := sn.createUnicitySeal(roundNumber, rootHash)
	if err != nil {
		return nil, err
	}
	cert, err := ut.GetCertificate(sdr.SystemIdentifier)
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

func (sn *SingleNodePartition) createUnicitySeal(roundNumber uint64, rootHash []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: roundNumber,
		Hash:                 rootHash,
	}
	return u, u.Sign("test", sn.rootSigner)
}

func (sn *SingleNodePartition) GetLatestBlock(t *testing.T) *block.Block {
	dbIt := sn.store.Last()
	defer func() {
		if err := dbIt.Close(); err != nil {
			logger.Warning("Unexpected DB iterator error %v", err)
		}
	}()
	var bl block.Block
	require.NoError(t, dbIt.Value(&bl))
	return &bl
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
	luc, found := sn.certs[p.SystemIdentifier(req.SystemIdentifier)]
	require.True(t, found)
	err := consensus.CheckBlockCertificationRequest(req, luc)
	require.NoError(t, err)
	uc, err := sn.CreateUnicityCertificate(req.InputRecord, sn.rootRound+1)
	require.NoError(t, err)
	// update state
	sn.rootRound = uc.UnicitySeal.RootChainRoundNumber
	sn.certs[p.SystemIdentifier(req.SystemIdentifier)] = uc
	return uc
}

func (sn *SingleNodePartition) SubmitT1Timeout(t *testing.T) {
	sn.eh.Reset()
	sn.partition.timers.C <- &timer.Task{}
	require.Eventually(t, func() bool {
		return len(sn.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
}

func (sn *SingleNodePartition) SubmitMonitorTimeout(t *testing.T) {
	t.Helper()
	sn.eh.Reset()
	sn.partition.handleMonitoring()
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

func (l *TestLeaderSelector) UpdateLeader(seal *certificates.UnicityCertificate) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if seal == nil {
		l.leader = ""
		return
	}
	l.leader = l.currentNode
}

func (l *TestLeaderSelector) GetLeaderID() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

func (l *TestLeaderSelector) LeaderFromUnicitySeal(seal *certificates.UnicityCertificate) peer.ID {
	if seal == nil {
		return ""
	}
	return l.currentNode
}

func createPeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{}
	// fake validator, so that network 'send' requests don't fail
	_, validatorPubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
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

func NextBlockReceived(t *testing.T, tp *SingleNodePartition, prevBlock *block.Block) func() bool {
	t.Helper()
	return func() bool {
		// Empty blocks are not persisted, assume new block is received if new last UC round is bigger than block UC round
		// NB! it could also be that repeat UC is received
		return tp.partition.luc.InputRecord.RoundNumber > prevBlock.UnicityCertificate.InputRecord.RoundNumber
	}
}

func ContainsTransaction(block *block.Block, tx *txsystem.Transaction) bool {
	for _, t := range block.Transactions {
		if reflect.DeepEqual(t, tx) {
			return true
		}
	}
	return false
}

func RequestReceived(tp *SingleNodePartition, req string) func() bool {
	return func() bool {
		messages := tp.mockNet.SentMessages(req)
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

func ContainsEventType(t *testing.T, tp *SingleNodePartition, evType event.Type) {
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == evType {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
