package rootchain

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/monolithic"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/rootchain/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var partitionID = types.SystemID([]byte{0, 0xFF, 0, 1})
var unknownID = []byte{0, 0, 0, 0}
var partitionInputRecord = &types.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

type MockConsensusManager struct {
	certReqCh    chan consensus.IRChangeRequest
	certResultCh chan *types.UnicityCertificate
	partitions   partitions.PartitionConfiguration
	certs        map[protocol.SystemIdentifier]*types.UnicityCertificate
}

func NewMockConsensus(rg *genesis.RootGenesis, partitionStore partitions.PartitionConfiguration) (*MockConsensusManager, error) {
	var c = make(map[protocol.SystemIdentifier]*types.UnicityCertificate)
	for _, partition := range rg.Partitions {
		c[partition.GetSystemIdentifierString()] = partition.Certificate
	}

	return &MockConsensusManager{
		// use buffered channels here, we just want to know if a tlg is received
		certReqCh:    make(chan consensus.IRChangeRequest, 1),
		certResultCh: make(chan *types.UnicityCertificate, 1),
		partitions:   partitionStore,
		certs:        c,
	}, nil
}

func (m *MockConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return m.certReqCh
}

func (m *MockConsensusManager) CertificationResult() <-chan *types.UnicityCertificate {
	return m.certResultCh
}

func (m *MockConsensusManager) Run(_ context.Context) error {
	// nothing to do
	return nil
}

func (m *MockConsensusManager) GetLatestUnicityCertificate(id protocol.SystemIdentifier) (*types.UnicityCertificate, error) {
	luc, f := m.certs[id]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

func initRootValidator(t *testing.T, net PartitionNet) (*Node, *testutils.TestNode, []*testutils.TestNode, *genesis.RootGenesis) {
	t.Helper()
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	node := testutils.NewTestNode(t)
	verifier := node.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := node.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitionStore, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	require.NoError(t, err)
	cm, err := NewMockConsensus(rootGenesis, partitionStore)
	require.NoError(t, err)
	validator, err := New(node.Peer, net, partitionStore, cm)
	require.NoError(t, err)
	require.NotNil(t, validator)
	return validator, node, partitionNodes, rootGenesis
}

func TestRootValidatorTest_ConstructWithMonolithicManager(t *testing.T) {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	node := testutils.NewTestNode(t)
	verifier := node.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := node.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	mockNet := testnetwork.NewMockNetwork()
	partitionStore, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	require.NoError(t, err)
	cm, err := monolithic.NewMonolithicConsensusManager(
		node.Peer.ID().String(),
		rootGenesis,
		partitionStore,
		node.Signer)
	require.NoError(t, err)
	validator, err := New(node.Peer, mockNet, partitionStore, cm)
	require.NoError(t, err)
	require.NotNil(t, validator)
}

func TestRootValidatorTest_CertificationReqRejected(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, unknownID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(context.Background(), req)
	// unknown id, gets rejected
	require.NotContains(t, rootValidator.incomingRequests.store, protocol.SystemIdentifier(unknownID))
	// unknown node gets rejected
	unknownNode := testutils.NewTestNode(t)
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, unknownNode)
	rootValidator.onBlockCertificationRequest(context.Background(), req)
	require.NotContains(t, rootValidator.incomingRequests.store, protocol.SystemIdentifier(partitionID))
	// signature does not verify
	invalidNode := testutils.TestNode{
		Peer:     partitionNodes[0].Peer,
		Signer:   unknownNode.Signer,
		Verifier: unknownNode.Verifier,
	}
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, &invalidNode)
	rootValidator.onBlockCertificationRequest(context.Background(), req)
	require.NotContains(t, rootValidator.incomingRequests.store, protocol.SystemIdentifier(partitionID))
}

func TestRootValidatorTest_CertificationReqEquivocatingReq(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(context.Background(), req)
	// request is accepted
	require.Contains(t, rootValidator.incomingRequests.store, protocol.SystemIdentifier(partitionID))
	equivocatingIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	eqReq := testutils.CreateBlockCertificationRequest(t, equivocatingIR, partitionID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(context.Background(), eqReq)
	buffer, f := rootValidator.incomingRequests.store[protocol.SystemIdentifier(partitionID)]
	require.True(t, f)
	storedNodeReqHash, f := buffer.nodeRequest[partitionNodes[0].Peer.ID().String()]
	require.True(t, f)
	require.EqualValues(t, sha256.Sum256(req.InputRecord.Bytes()), storedNodeReqHash)
	require.Len(t, buffer.requests, 1)
	for _, certReq := range buffer.requests {
		require.Len(t, certReq, 1)
	}
}

func TestRootValidatorTest_SimulateNetCommunication(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].Peer.ID(), network.ProtocolBlockCertification, req)
	// since consensus is simple majority, then consensus is now achieved and message should be forwarded
	require.Eventually(t, func() bool { return len(rootValidator.consensusManager.RequestCertification()) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationNoQuorum(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR1 := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req1 := testutils.CreateBlockCertificationRequest(t, newIR1, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req1)
	newIR2 := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req2 := testutils.CreateBlockCertificationRequest(t, newIR2, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].Peer.ID(), network.ProtocolBlockCertification, req2)
	newIR3 := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req3 := testutils.CreateBlockCertificationRequest(t, newIR3, partitionID, partitionNodes[2])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[2].Peer.ID(), network.ProtocolBlockCertification, req3)
	// no consensus can be achieved all reported different hashes
	require.Eventually(t, func() bool { return len(rootValidator.consensusManager.RequestCertification()) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationHandshake(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create
	h := &handshake.Handshake{
		SystemIdentifier: partitionID,
		NodeIdentifier:   partitionNodes[1].Peer.ID().String(),
	}
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolHandshake, h)
	// make sure certificate is sent in return
	testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	// make sure that the node is subscribed
	subscribed := rootValidator.subscription.Get(protocol.SystemIdentifier(partitionID))
	require.Len(t, subscribed, 1)
	require.Equal(t, partitionNodes[1].Peer.ID().String(), subscribed[0])
	// set network in error state
	mockNet.SetErrorState(fmt.Errorf("failed to dial"))
	// simulate root response, which will fail to send due to network error
	// create certification request
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	uc := &types.UnicityCertificate{
		InputRecord: newIR,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier: partitionID,
		},
		UnicitySeal: &types.UnicitySeal{},
	}
	rootValidator.onCertificationResult(ctx, uc)
	rootValidator.onCertificationResult(ctx, uc)
	// two send errors, but node is still subscribed
	subscribed = rootValidator.subscription.Get(protocol.SystemIdentifier(partitionID))
	require.Len(t, subscribed, 1)
	rootValidator.onCertificationResult(ctx, uc)
	// on third error subscription is cleared
	subscribed = rootValidator.subscription.Get(protocol.SystemIdentifier(partitionID))
	require.Len(t, subscribed, 0)
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidReqRoundNumber(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootValidator)
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidHash(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootValidator)
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{
		PreviousHash: test.RandomBytes(32),
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateResponse(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	uc := &types.UnicityCertificate{
		InputRecord: newIR,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier: partitionID,
		},
		UnicitySeal: &types.UnicitySeal{},
	}
	// simulate 2x subscriptions
	rootValidator.subscription.Subscribe(protocol.SystemIdentifier(rg.Partitions[0].SystemDescriptionRecord.SystemIdentifier), rg.Partitions[0].Nodes[0].NodeIdentifier)
	rootValidator.subscription.Subscribe(protocol.SystemIdentifier(rg.Partitions[0].SystemDescriptionRecord.SystemIdentifier), rg.Partitions[0].Nodes[1].NodeIdentifier)
	// simulate response from consensus manager
	rootValidator.onCertificationResult(ctx, uc)
	// UC's are sent to all partition nodes
	certs := testutils.MockNetAwaitMultiple[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates, 2)
	require.Len(t, certs, 2)
	for _, cert := range certs {
		require.Equal(t, partitionID, cert.UnicityTreeCertificate.SystemIdentifier)
		require.Equal(t, newIR, cert.InputRecord)
	}
}

func TestRootValidator_ResultUnknown(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, _, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	uc := &types.UnicityCertificate{
		InputRecord: newIR,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier: unknownID,
		},
		UnicitySeal: &types.UnicitySeal{},
	}
	// simulate response from consensus manager
	rootValidator.onCertificationResult(ctx, uc)
	// no responses will be sent
	require.Empty(t, mockNet.SentMessages(network.ProtocolUnicityCertificates))
}

func TestRootValidator_ExitWhenPendingCertRequestAndCMClosed(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	mockRunFn := func(ctx context.Context) error {
		g, gctx := errgroup.WithContext(ctx)
		// Start receiving messages from partition nodes
		g.Go(func() error { return rootValidator.loop(gctx) })
		// Start handling certification responses
		g.Go(func() error { return rootValidator.handleConsensus(gctx) })
		return g.Wait()
	}
	go func() { require.ErrorIs(t, mockRunFn(ctx), context.Canceled) }()
	// create certification request
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].Peer.ID(), network.ProtocolBlockCertification, req)
	// node should still exit normally even if CM loop is not running and reading the channel
}
