package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
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
	certs      map[types.SystemID]*types.UnicityCertificate
	rootSigner crypto.Signer
	mockNet    *testnetwork.MockNet
	eh         *testevent.TestEventHandler
	obs        Observability
}

type partitionStartupDependencies struct {
	peerConf    *network.PeerConfiguration
	txSystem    txsystem.TransactionSystem
	nodeSigner  crypto.Signer
	genesis     *genesis.PartitionGenesis
	trustBase   types.RootTrustBase
	network     ValidatorNetwork
	nodeOptions []NodeOption
}

func (t *AlwaysValidTransactionValidator) Validate(_ *types.TransactionOrder, _ uint64) error {
	return nil
}

func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

func SetupNewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	peerConf := createPeerConfiguration(t)
	pdr := types.PartitionDescriptionRecord{Version: 1, NetworkIdentifier: 5, SystemIdentifier: 0x01010101, TypeIdLen: 8, UnitIdLen: 256, T2Timeout: 2500 * time.Millisecond}
	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)
	nodeGenesis, err := NewNodeGenesis(
		// Should actually create the genesis state before the
		// txSystem and start the txSystem with it. Works like
		// this if the txSystem has empty state as well.
		state.NewEmptyState(),
		pdr,
		WithPeerID(peerConf.ID),
		WithSigningKey(nodeSigner),
		WithEncryptionPubKey(peerConf.KeyPair.PublicKey),
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
	trustBase, err := rootGenesis.GenerateTrustBase()
	require.NoError(t, err)

	require.NoError(t, txSystem.Commit(partitionGenesis[0].Certificate))

	// root state
	var certs = make(map[types.SystemID]*types.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		certs[partition.GetSystemDescriptionRecord().GetSystemIdentifier()] = partition.Certificate
	}

	net := testnetwork.NewMockNetwork(t)

	// allows restarting the node
	deps := &partitionStartupDependencies{
		peerConf:    peerConf,
		txSystem:    txSystem,
		nodeSigner:  nodeSigner,
		genesis:     partitionGenesis[0],
		trustBase:   trustBase,
		network:     net,
		nodeOptions: nodeOptions,
	}

	obs := testobserve.Default(t)
	partition := &SingleNodePartition{
		nodeDeps:   deps,
		rootRound:  rootGenesis.GetRoundNumber(),
		certs:      certs,
		rootSigner: rootSigner,
		mockNet:    net,
		eh:         &testevent.TestEventHandler{},
		obs:        observability.WithLogger(obs, obs.Logger().With(logger.NodeID(peerConf.ID))),
		nodeConf:   &configuration{},
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

	partition := SetupNewSingleNodePartition(t, txSystem, nodeOptions...)
	done := StartSingleNodePartition(ctx, t, partition)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	return partition
}

func (sn *SingleNodePartition) newNode() error {
	nodeID := sn.nodeDeps.peerConf.ID

	n, err := NewNode(
		context.Background(),
		sn.nodeDeps.peerConf,
		sn.nodeDeps.nodeSigner,
		sn.nodeDeps.txSystem,
		sn.nodeDeps.genesis,
		sn.nodeDeps.trustBase,
		sn.nodeDeps.network,
		sn.obs,
		append([]NodeOption{
			WithT1Timeout(100 * time.Minute),
			WithLeaderSelector(&TestLeaderSelector{
				leader:      nodeID,
				currentNode: nodeID,
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
	sn.mockNet.AddTransaction(context.Background(), tx)
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
	sdr := sn.nodeDeps.genesis.PartitionDescription
	sdrHash := sdr.Hash(gocrypto.SHA256)
	data := []*types.UnicityTreeData{{
		SystemIdentifier:         sdr.SystemIdentifier,
		InputRecord:              ir,
		PartitionDescriptionHash: sdrHash,
	},
	}
	ut, err := unicitytree.New(gocrypto.SHA256, data)
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

	return &types.UnicityCertificate{Version: 1,
		InputRecord: ir,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{Version: 1,
			SystemIdentifier:         cert.SystemIdentifier,
			HashSteps:                cert.HashSteps,
			PartitionDescriptionHash: sdrHash,
		},
		UnicitySeal: unicitySeal,
	}, nil
}

func (sn *SingleNodePartition) createUnicitySeal(roundNumber uint64, rootHash []byte) (*types.UnicitySeal, error) {
	u := &types.UnicitySeal{Version: 1,
		RootChainRoundNumber: roundNumber,
		Timestamp:            types.NewTimestamp(),
		Hash:                 rootHash,
	}
	return u, u.Sign("test", sn.rootSigner)
}

func (sn *SingleNodePartition) GetCommittedUC(t *testing.T) *types.UnicityCertificate {
	uc := sn.nodeDeps.txSystem.CommittedUC()
	require.NotNil(t, uc)
	return uc
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
	luc, found := sn.certs[req.Partition]
	require.True(t, found)
	require.NoError(t, consensus.CheckBlockCertificationRequest(req, luc))
	uc, err := sn.CreateUnicityCertificate(req.InputRecord, sn.rootRound+1)
	require.NoError(t, err)
	// update state
	sn.rootRound = uc.UnicitySeal.RootChainRoundNumber
	sn.certs[req.Partition] = uc
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
	sn.partition.handleMonitoring(context.Background(), time.Now().Add(-3*sn.nodeConf.GetT2Timeout()), time.Now())
}

type TestLeaderSelector struct {
	leader      peer.ID
	currentNode peer.ID
	mutex       sync.Mutex
}

// IsLeader returns true it current node is the leader and must propose the next block.
func (l *TestLeaderSelector) IsLeader(peerID peer.ID) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader == peerID
}

func (l *TestLeaderSelector) UpdateLeader(seal *types.UnicityCertificate, validators []peer.ID) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if seal == nil {
		l.leader = ""
		return
	}
	l.leader = l.currentNode
}

func (l *TestLeaderSelector) GetLeader() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

func (l *TestLeaderSelector) LeaderFunc(seal *types.UnicityCertificate, validators []peer.ID) peer.ID {
	if seal == nil {
		return ""
	}
	return l.currentNode
}

func createPeerConfiguration(t *testing.T) *network.PeerConfiguration {
	// fake validator, so that network 'send' requests don't fail
	_, fakeValidatorPubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
	fakeValidatorID, err := peer.IDFromPublicKey(fakeValidatorPubKey)
	require.NoError(t, err)

	privKey, pubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)

	privKeyBytes, err := privKey.Raw()
	require.NoError(t, err)

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(t, err)

	peerID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)

	peerConf, err := network.NewPeerConfiguration(
		"/ip4/127.0.0.1/tcp/0",
		nil,
		&network.PeerKeyPair{PublicKey: pubKeyBytes, PrivateKey: privKeyBytes},
		nil,
		// Need to also add peerID to make it a validator node.
		[]peer.ID{fakeValidatorID, peerID},
	)
	require.NoError(t, err)

	return peerConf
}

func NextBlockReceived(t *testing.T, tp *SingleNodePartition, committedUC *types.UnicityCertificate) func() bool {
	t.Helper()
	return func() bool {
		// Empty blocks are not persisted, assume new block is received if new last UC round is bigger than block UC round
		// NB! it could also be that repeat UC is received
		return tp.partition.luc.Load().GetRoundNumber() > committedUC.GetRoundNumber()
	}
}

func ContainsTransaction(block *types.Block, tx *types.TransactionOrder) bool {
	for _, t := range block.Transactions {
		txBytes, err := tx.MarshalCBOR()
		if err != nil {
			panic(err)
		}
		if bytes.Equal(t.TransactionOrder, txBytes) {
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
