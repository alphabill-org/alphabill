package rootchain

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testuc "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobservability "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const partitionID types.PartitionID = 0x00FF0001
const unknownPartitionID types.PartitionID = 0

type MockConsensusManager struct {
	certReqCh    chan consensus.IRChangeRequest
	certResultCh chan *certification.CertificationResponse
	certs        map[types.PartitionShardID]*certification.CertificationResponse
	shardInfo    map[types.PartitionShardID]*storage.ShardInfo
}

func NewMockConsensus(t *testing.T, rootNode *testutils.TestNode, shardConf *types.PartitionDescriptionRecord) (*MockConsensusManager, error) {
	psID := types.PartitionShardID{PartitionID: shardConf.PartitionID, ShardID: shardConf.ShardID.Key()}

	var certs = make(map[types.PartitionShardID]*certification.CertificationResponse)
	certs[psID] = createInitialCR(t, shardConf, shardConf.Validators[0].NodeID, rootNode.Signer)

	shardInfo := map[types.PartitionShardID]*storage.ShardInfo{}
	si, err := storage.NewShardInfo(shardConf, gocrypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("creating shard info: %w", err)
	}
	si.LastCR = certs[psID]
	shardInfo[psID] = si

	return &MockConsensusManager{
		// use buffered channels here, we just want to know if a tlg is received
		certReqCh:    make(chan consensus.IRChangeRequest, 1),
		certResultCh: make(chan *certification.CertificationResponse, 1),
		certs:        certs,
		shardInfo:    shardInfo,
	}, nil
}

func (m *MockConsensusManager) RequestCertification(ctx context.Context, cr consensus.IRChangeRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.certReqCh <- cr:
	}
	return nil
}

func (m *MockConsensusManager) CertificationResult() <-chan *certification.CertificationResponse {
	return m.certResultCh
}

func (m *MockConsensusManager) Run(_ context.Context) error {
	// nothing to do
	return nil
}

func (m *MockConsensusManager) GetLatestUnicityCertificate(partitionID types.PartitionID, shardID types.ShardID) (*certification.CertificationResponse, error) {
	psID := types.PartitionShardID{PartitionID: partitionID, ShardID: shardID.Key()}
	luc, f := m.certs[psID]
	if !f {
		return nil, fmt.Errorf("no certificate found for partition id %X", partitionID)
	}
	return luc, nil
}

func (m *MockConsensusManager) ShardInfo(partitionID types.PartitionID, shardID types.ShardID) (*storage.ShardInfo, error) {
	psID := types.PartitionShardID{PartitionID: partitionID, ShardID: shardID.Key()}
	if si, ok := m.shardInfo[psID]; ok {
		return si, nil
	}
	return nil, fmt.Errorf("no ShardInfo for %s - %s", partitionID, shardID)
}

func initRootNode(t *testing.T, net PartitionNet) (*Node, *testutils.TestNode, []*testutils.TestNode, *types.PartitionDescriptionRecord) {
	t.Helper()
	rootNode := testutils.NewTestNode(t)
	shardNodes, shardNodeInfos := testutils.CreateTestNodes(t, 3)

	var shardConf = &types.PartitionDescriptionRecord{
		Version:         1,
		PartitionID:     partitionID,
		PartitionTypeID: 123,
		ShardID:         types.ShardID{},
		Validators:      shardNodeInfos,
	}

	cm, err := NewMockConsensus(t, rootNode, shardConf)
	require.NoError(t, err)

	observe := testobservability.Default(t)
	peer := peer.CreatePeer(t, rootNode.PeerConf)

	node, err := New(peer, net, cm, observability.WithLogger(observe, observe.Logger().With(logger.NodeID(rootNode.PeerConf.ID))))
	require.NoError(t, err)
	require.NotNil(t, node)
	return node, rootNode, shardNodes, shardConf
}

func TestNew_OK(t *testing.T) {
	rootNode1 := testutils.NewTestNode(t)
	rootNode2 := testutils.NewTestNode(t)
	trustBase := trustbase.NewTrustBase(t, rootNode1.Verifier, rootNode2.Verifier)

	partitionNetMock := testnetwork.NewMockNetwork(t)
	rootNetMock := testnetwork.NewMockNetwork(t)

	log := testlogger.New(t).With(logger.NodeID(rootNode1.PeerConf.ID))
	observe := observability.WithLogger(testobservability.Default(t), log)

	cm, err := consensus.NewConsensusManager(
		rootNode1.PeerConf.ID,
		trustBase,
		testpartition.NewOrchestration(t, log),
		rootNetMock,
		rootNode1.Signer,
		observe)
	require.NoError(t, err)

	peer := peer.CreatePeer(t, rootNode1.PeerConf)
	validator, err := New(peer, partitionNetMock, cm, observe)
	require.NoError(t, err)
	require.NotNil(t, validator)
}

func TestRootValidatorTest_CertificationReqRejected(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, _, shardNodeConfs, _ := initRootNode(t, mockNet)
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1, 2, 3},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, unknownPartitionID, shardNodeConfs[0])
	require.ErrorContains(t, rootNode.onBlockCertificationRequest(context.Background(), req), "acquiring shard")
	// unknown partition shard, gets rejected
	require.NotContains(t, rootNode.incomingRequests.store, unknownPartitionID)

	// unknown node gets rejected
	unknownNode := testutils.NewTestNode(t)
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, unknownNode)
	require.ErrorContains(t, rootNode.onBlockCertificationRequest(context.Background(), req), fmt.Sprintf("node %q is not in the trustbase of the shard", unknownNode.PeerConf.ID))
	require.NotContains(t, rootNode.incomingRequests.store, partitionID)

	// signature does not verify
	invalidNode := testutils.TestNode{
		PeerConf: shardNodeConfs[0].PeerConf,
		Signer:   unknownNode.Signer,
		Verifier: shardNodeConfs[0].Verifier,
	}
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, &invalidNode)
	require.EqualError(t, rootNode.onBlockCertificationRequest(context.Background(), req), `invalid block certification request: invalid certification request: signature verification: verification failed`)
	require.NotContains(t, rootNode.incomingRequests.store, partitionID)
}

func TestRootValidatorTest_CertificationReqEquivocatingReq(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, _, shardNodeConfs, _ := initRootNode(t, mockNet)
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[0])
	require.NoError(t, rootNode.onBlockCertificationRequest(context.Background(), req))
	// request is accepted
	key := partitionShard{partition: partitionID, shard: req.ShardID.Key()}
	require.Contains(t, rootNode.incomingRequests.store, key)

	equivocatingIR := newIR.NewRepeatIR()
	equivocatingIR.Hash = test.RandomBytes(32)
	eqReq := testutils.CreateBlockCertificationRequest(t, equivocatingIR, partitionID, shardNodeConfs[0])
	require.ErrorContains(t, rootNode.onBlockCertificationRequest(context.Background(), eqReq), "request of the node in this round already stored")

	buffer, f := rootNode.incomingRequests.store[key]
	require.True(t, f, "no requests for %#v", key)
	require.Contains(t, buffer.nodeRequest, shardNodeConfs[0].PeerConf.ID.String())
	require.Len(t, buffer.requests, 1)
	for _, certReq := range buffer.requests {
		require.Len(t, certReq, 1)
	}
}

func TestRootValidatorTest_SimulateNetCommunication(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())
	// create certification request
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[0])
	mockNet.WaitReceive(t, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[1])
	mockNet.WaitReceive(t, req)
	// since consensus is simple majority, then consensus is now achieved and message should be forwarded
	mcm := rootNode.consensusManager.(*MockConsensusManager)
	require.Eventually(t, func() bool { return len(mcm.certReqCh) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationNoQuorum(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())

	// create certification request
	newIR1 := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req1 := testutils.CreateBlockCertificationRequest(t, newIR1, partitionID, shardNodeConfs[0])
	mockNet.WaitReceive(t, req1)
	newIR2 := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req2 := testutils.CreateBlockCertificationRequest(t, newIR2, partitionID, shardNodeConfs[1])
	mockNet.WaitReceive(t, req2)
	newIR3 := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req3 := testutils.CreateBlockCertificationRequest(t, newIR3, partitionID, shardNodeConfs[2])
	mockNet.WaitReceive(t, req3)
	// no consensus can be achieved all reported different hashes
	mcm := rootNode.consensusManager.(*MockConsensusManager)
	require.Eventually(t, func() bool { return len(mcm.certReqCh) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationHandshake(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, _ := initRootNode(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())
	// create
	h := &handshake.Handshake{
		PartitionID: partitionID,
		ShardID:     types.ShardID{},
		NodeID:      shardNodeConfs[0].PeerConf.ID.String(),
	}
	mockNet.WaitReceive(t, h)

	// make sure certificate is sent in return
	cr := testutils.MockAwaitMessage[*certification.CertificationResponse](t, mockNet, network.ProtocolUnicityCertificates)

	// make sure the node is subscribed - first handshake after shard activation subscribes the node
	subscribed := rootNode.subscription.Get(partitionID)
	require.Contains(t, subscribed, shardNodeConfs[0].PeerConf.ID.String())

	// set network in error state
	mockNet.SetErrorState(fmt.Errorf("failed to dial"))

	rootNode.onCertificationResult(ctx, cr)
	subscribed = rootNode.subscription.Get(partitionID)
	require.Contains(t, subscribed, shardNodeConfs[0].PeerConf.ID.String())
	rootNode.onCertificationResult(ctx, cr)

	// subscription is cleared, node got two responses and is required to issue a block certification request
	require.Empty(t, rootNode.subscription.Get(partitionID))
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidReqRoundNumber(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootNode)
	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: []byte{1},
		RoundNumber:  2, // round 1 is expected after shard activation
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[0])
	mockNet.WaitReceive(t, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*certification.CertificationResponse](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, uint64(0), repeatCert.UC.GetRoundNumber())
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidHash(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootNode)
	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{
		Version:      1,
		PreviousHash: test.RandomBytes(32),
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[0])
	mockNet.WaitReceive(t, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*certification.CertificationResponse](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, uint64(0), repeatCert.UC.GetRoundNumber())
}

func TestRootValidatorTest_SimulateResponse(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, rootNodeConf, shardNodeConfs, shardConf := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	require.Len(t, shardNodeConfs, 3)
	require.NotEmpty(t, rootNodeConf.PeerConf.ID.String())

	// create certification request
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  2,
	}
	cr := certification.CertificationResponse{
		Partition: partitionID,
		UC: types.UnicityCertificate{
			Version:     1,
			InputRecord: newIR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: partitionID,
			},
			UnicitySeal: &types.UnicitySeal{Version: 1},
		},
	}
	require.NoError(t,
		cr.SetTechnicalRecord(certification.TechnicalRecord{
			Round:    3,
			Epoch:    1,
			Leader:   shardConf.Validators[0].NodeID,
			StatHash: []byte{1},
			FeeHash:  []byte{2},
		}))

	// simulate 2x subscriptions
	id32 := shardConf.PartitionID
	rootNode.subscription.Subscribe(id32, shardConf.Validators[0].NodeID)
	rootNode.subscription.Subscribe(id32, shardConf.Validators[1].NodeID)

	// simulate response from consensus manager
	rootNode.onCertificationResult(ctx, &cr)

	// UC's are sent to all partition nodes
	certs := testutils.MockNetAwaitMultiple[*certification.CertificationResponse](t, mockNet, network.ProtocolUnicityCertificates, 2)
	require.Len(t, certs, 2)
	for _, cert := range certs {
		require.Equal(t, partitionID, cert.UC.UnicityTreeCertificate.Partition)
		require.Equal(t, newIR, cert.UC.InputRecord)
	}
}

func TestRootValidator_ResultUnknown(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, _, _, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootNode.Run(ctx), context.Canceled) }()

	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  2,
	}
	cr := certification.CertificationResponse{
		Partition: partitionID,
		UC: types.UnicityCertificate{
			Version:     1,
			InputRecord: newIR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				Version:   1,
				Partition: unknownPartitionID,
			},
			UnicitySeal: &types.UnicitySeal{Version: 1},
		},
	}
	// simulate response from consensus manager
	rootNode.onCertificationResult(ctx, &cr)
	// no responses will be sent
	require.Empty(t, mockNet.SentMessages(network.ProtocolUnicityCertificates))
}

func TestRootValidator_ExitWhenPendingCertRequestAndCMClosed(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootNode, _, shardNodeConfs, _ := initRootNode(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)

	mockRunFn := func(ctx context.Context) error {
		g, gctx := errgroup.WithContext(ctx)
		// Start receiving messages from partition nodes
		g.Go(func() error { return rootNode.loop(gctx) })
		// Start handling certification responses
		g.Go(func() error { return rootNode.handleConsensus(gctx) })
		return g.Wait()
	}

	go func() { require.ErrorIs(t, mockRunFn(ctx), context.Canceled) }()

	// create certification request
	newIR := &types.InputRecord{
		Version:      1,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{1},
		RoundNumber:  1,
		Timestamp:    lastUCTimestamp(t, rootNode),
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[0])
	mockNet.WaitReceive(t, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, shardNodeConfs[1])
	mockNet.WaitReceive(t, req)
	// consensus is achieved and request will be sent to CM, but CM is not running
	// node should still exit normally even if CM loop is not running and reading the channel
}

func createInitialCR(t *testing.T, shardConf *types.PartitionDescriptionRecord, leader string, signer crypto.Signer) *certification.CertificationResponse {
	tr := certification.TechnicalRecord{
		Round:    1,
		Epoch:    0,
		Leader:   leader,
		StatHash: []byte{1},
		FeeHash:  []byte{2},
	}
	trHash, err := tr.Hash()
	require.NoError(t, err)
	uc := testuc.CreateUnicityCertificate(
		t,
		signer,
		&types.InputRecord{Version: 1},
		shardConf,
		rctypes.GenesisRootRound,
		make([]byte, 32),
		trHash)

	cr := &certification.CertificationResponse{
		Partition: shardConf.PartitionID,
		Shard:     shardConf.ShardID,
		UC:        *uc,
		Technical: tr,
	}
	return cr
}

func lastUCTimestamp(t *testing.T, rootNode *Node) uint64 {
	si, err := rootNode.consensusManager.ShardInfo(partitionID, types.ShardID{})
	require.NoError(t, err)
	return si.LastCR.UC.UnicitySeal.Timestamp
}
