package partition

import (
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

type SingleNodePartition struct {
	nodeConf   *configuration
	store      keyvaluedb.KeyValueDB
	partition  *Node
	nodeDeps   *partitionStartupDependencies
	rootRound  uint64
	certs      map[types.SystemID32]*types.UnicityCertificate
	rootSigner crypto.Signer
	mockNet    *testnetwork.MockNet
	eh         *testevent.TestEventHandler
	log        *slog.Logger
}

type partitionStartupDependencies struct {
	peer        *network.Peer
	txSystem    txsystem.TransactionSystem
	nodeSigner  crypto.Signer
	genesis     *genesis.PartitionGenesis
	net         Net
	nodeOptions []NodeOption
}

func (t *AlwaysValidTransactionValidator) Validate(_ *types.TransactionOrder, _ uint64) error {
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
	require.NoError(t, err)

	// root state
	var certs = make(map[types.SystemID32]*types.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		id32, err := partition.GetSystemDescriptionRecord().GetSystemIdentifier().Id32()
		require.NoError(t, err)
		certs[id32] = partition.Certificate
	}

	net := testnetwork.NewMockNetwork()

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
		eh:         &testevent.TestEventHandler{},
		log:        testlogger.New(t).With(logger.NodeID(peer.ID())),
	}
	return partition
}

func StartSingleNodePartition(ctx context.Context, t *testing.T, p *SingleNodePartition) chan struct{} {
	// partition node
	require.NoError(t, p.newNode(), "failed to init partition node")
	done := make(chan struct{})
	go func() {
		require.ErrorIs(t, p.partition.Run(ctx), context.Canceled)
		close(done)
	}()
	return done
}

func RunSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	partition := SetupNewSingleNodePartition(t, txSystem, nodeOptions...)
	StartSingleNodePartition(ctx, t, partition)
	return partition
}

func (sn *SingleNodePartition) newNode() error {
	n, err := New(
		sn.nodeDeps.peer,
		sn.nodeDeps.nodeSigner,
		sn.nodeDeps.txSystem,
		sn.nodeDeps.genesis,
		sn.nodeDeps.net,
		sn.log,
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

func (sn *SingleNodePartition) SubmitTx(tx *types.TransactionOrder) error {
	sn.mockNet.Receive(tx)
	return nil
}

func (sn *SingleNodePartition) SubmitTxFromRPC(tx *types.TransactionOrder) error {
	_, err := sn.partition.SubmitTx(context.Background(), tx)
	return err
}

func (sn *SingleNodePartition) SubmitUnicityCertificate(uc *types.UnicityCertificate) {
	sn.mockNet.Receive(uc)

}

func (sn *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) {
	sn.mockNet.Receive(prop)
}

func (sn *SingleNodePartition) CreateUnicityCertificate(ir *types.InputRecord, roundNumber uint64) (*types.UnicityCertificate, error) {
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

	return &types.UnicityCertificate{
		InputRecord: ir,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier:      cert.SystemIdentifier,
			SiblingHashes:         cert.SiblingHashes,
			SystemDescriptionHash: sdrHash,
		},
		UnicitySeal: unicitySeal,
	}, nil
}

func (sn *SingleNodePartition) createUnicitySeal(roundNumber uint64, rootHash []byte) (*types.UnicitySeal, error) {
	u := &types.UnicitySeal{
		RootChainRoundNumber: roundNumber,
		Timestamp:            util.MakeTimestamp(),
		Hash:                 rootHash,
	}
	return u, u.Sign("test", sn.rootSigner)
}

func (sn *SingleNodePartition) GetLatestBlock(t *testing.T) *types.Block {
	dbIt := sn.store.Last()
	defer func() {
		if err := dbIt.Close(); err != nil {
			t.Errorf("Unexpected DB iterator error: %v", err)
		}
	}()
	var bl types.Block
	require.NoError(t, dbIt.Value(&bl))
	return &bl
}

func (sn *SingleNodePartition) CreateBlock(t *testing.T) {
	sn.SubmitT1Timeout(t)
	sn.eh.Reset()
	sn.SubmitUnicityCertificate(sn.IssueBlockUC(t))
	testevent.ContainsEvent(t, sn.eh, event.BlockFinalized)
}

func (sn *SingleNodePartition) IssueBlockUC(t *testing.T) *types.UnicityCertificate {
	req := sn.mockNet.SentMessages(network.ProtocolBlockCertification)[0].Message.(*certification.BlockCertificationRequest)
	sn.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	id32, err := req.SystemIdentifier.Id32()
	require.NoError(t, err)
	luc, found := sn.certs[id32]
	require.True(t, found)
	require.NoError(t, consensus.CheckBlockCertificationRequest(req, luc))
	uc, err := sn.CreateUnicityCertificate(req.InputRecord, sn.rootRound+1)
	require.NoError(t, err)
	// update state
	sn.rootRound = uc.UnicitySeal.RootChainRoundNumber
	sn.certs[id32] = uc
	return uc
}

func (sn *SingleNodePartition) SubmitT1Timeout(t *testing.T) {
	sn.eh.Reset()
	sn.partition.handleT1TimeoutEvent(context.Background())
	require.Eventually(t, func() bool {
		return len(sn.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
}

func (sn *SingleNodePartition) SubmitMonitorTimeout(t *testing.T) {
	t.Helper()
	sn.eh.Reset()
	sn.partition.handleMonitoring(context.Background(), time.Now().Add(-3*sn.nodeConf.GetT2Timeout()))
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

func (l *TestLeaderSelector) UpdateLeader(seal *types.UnicityCertificate) {
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

func (l *TestLeaderSelector) LeaderFunc(seal *types.UnicityCertificate) peer.ID {
	if seal == nil {
		return ""
	}
	return l.currentNode
}

func createPeer(t *testing.T) *network.Peer {
	// fake validator, so that network 'send' requests don't fail
	_, pubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
	fakeValidatorID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)

	conf := &network.PeerConfiguration{
		//KeyPair will be generated
		Validators: []peer.ID{fakeValidatorID},
		Address:    "/ip4/127.0.0.1/tcp/0",
	}
	newPeer, err := network.NewPeer(context.Background(), conf, testlogger.New(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, newPeer.Close()) })

	return newPeer
}

func NextBlockReceived(t *testing.T, tp *SingleNodePartition, prevBlock *types.Block) func() bool {
	t.Helper()
	return func() bool {
		// Empty blocks are not persisted, assume new block is received if new last UC round is bigger than block UC round
		// NB! it could also be that repeat UC is received
		return tp.partition.luc.Load().InputRecord.RoundNumber > prevBlock.UnicityCertificate.InputRecord.RoundNumber
	}
}

func ContainsTransaction(block *types.Block, tx *types.TransactionOrder) bool {
	for _, t := range block.Transactions {
		if reflect.DeepEqual(t.TransactionOrder, tx) {
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

// WaitNodeRequestReceived waits for req type message from node and if there is more than one, copy of the latest is
// returned and the buffer is cleared. NB! if there is already such a message received the method will return with the latest
// immediately. Make sure to clear the "sent" messages if test expects a new message.
func WaitNodeRequestReceived(t *testing.T, tp *SingleNodePartition, req string) *testnetwork.PeerMessage {
	t.Helper()
	defer tp.mockNet.ResetSentMessages(req)
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(req)
		return len(reqs) > 0
	}, test.WaitDuration, test.WaitTick)
	// if more than one return last, but there has to be at least one, otherwise require.Eventually fails before
	return &testnetwork.PeerMessage{
		ID:      reqs[len(reqs)-1].ID,
		Message: reqs[len(reqs)-1].Message,
	}
}
