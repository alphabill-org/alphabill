package rootchain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testgenesis "github.com/alphabill-org/alphabill/internal/testutils/genesis"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobservability "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc"
	"github.com/alphabill-org/alphabill/rootchain/consensus/monolithic"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const partitionID types.SystemID = 0x00FF0001
const unknownID types.SystemID = 0

var partitionInputRecord = &types.InputRecord{Version: 1,
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

type MockConsensusManager struct {
	certReqCh    chan consensus.IRChangeRequest
	certResultCh chan *certification.CertificationResponse
	partitions   partitions.PartitionConfiguration
	certs        map[types.SystemID]*certification.CertificationResponse
}

func NewMockConsensus(rg *genesis.RootGenesis, partitionStore partitions.PartitionConfiguration) (*MockConsensusManager, error) {
	var c = make(map[types.SystemID]*certification.CertificationResponse)
	for _, partition := range rg.Partitions {
		c[partition.PartitionDescription.GetSystemIdentifier()] = &certification.CertificationResponse{
			Partition: partition.PartitionDescription.GetSystemIdentifier(),
			UC:        *partition.Certificate,
		}
	}

	return &MockConsensusManager{
		// use buffered channels here, we just want to know if a tlg is received
		certReqCh:    make(chan consensus.IRChangeRequest, 1),
		certResultCh: make(chan *certification.CertificationResponse, 1),
		partitions:   partitionStore,
		certs:        c,
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

func (m *MockConsensusManager) GetLatestUnicityCertificate(id types.SystemID, shard types.ShardID) (*certification.CertificationResponse, error) {
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
	id := node.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitionStore, err := partitions.NewPartitionStore(testgenesis.NewGenesisStore(rootGenesis))
	require.NoError(t, err)
	require.NoError(t, partitionStore.Reset(func() uint64 { return 1 }))
	cm, err := NewMockConsensus(rootGenesis, partitionStore)
	require.NoError(t, err)

	observe := testobservability.Default(t)
	p := peer.CreatePeer(t, node.PeerConf)
	validator, err := New(p, net, partitionStore, cm, observability.WithLogger(observe, observe.Logger().With(logger.NodeID(id))))
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
	id := node.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	mockNet := testnetwork.NewMockNetwork(t)
	partitionStore, err := partitions.NewPartitionStore(testgenesis.NewGenesisStore(rootGenesis))
	require.NoError(t, err)
	require.NoError(t, partitionStore.Reset(func() uint64 { return 1 }))
	log := testlogger.New(t).With(logger.NodeID(id))
	trustBase, err := rootGenesis.GenerateTrustBase()
	require.NoError(t, err)
	cm, err := monolithic.NewMonolithicConsensusManager(
		node.PeerConf.ID.String(),
		trustBase,
		rootGenesis,
		partitionStore,
		node.Signer,
		log,
	)
	require.NoError(t, err)

	observe := testobservability.Default(t)
	p := peer.CreatePeer(t, node.PeerConf)
	validator, err := New(p, mockNet, partitionStore, cm, observability.WithLogger(observe, observe.Logger().With(logger.NodeID(id))))
	require.NoError(t, err)
	require.NotNil(t, validator)
}

func TestRootValidatorTest_ConstructWithDistributedManager(t *testing.T) {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	node := testutils.NewTestNode(t)
	verifier := node.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := node.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitionNetMock := testnetwork.NewMockNetwork(t)
	rootHost := testutils.NewTestNode(t)
	rootNetMock := testnetwork.NewMockNetwork(t)
	partitionStore, err := partitions.NewPartitionStore(testgenesis.NewGenesisStore(rootGenesis))
	require.NoError(t, err)
	require.NoError(t, partitionStore.Reset(func() uint64 { return 1 }))
	trustBase, err := createTrustBaseFromRootGenesis(rootGenesis)
	require.NoError(t, err)
	obs := testobservability.Default(t)
	observe := observability.WithLogger(obs, obs.Logger().With(logger.NodeID(id)))
	cm, err := abdrc.NewDistributedAbConsensusManager(rootHost.PeerConf.ID,
		rootGenesis,
		trustBase,
		partitionStore,
		rootNetMock,
		rootHost.Signer,
		observe)
	require.NoError(t, err)
	p := peer.CreatePeer(t, node.PeerConf)
	validator, err := New(p, partitionNetMock, partitionStore, cm, observe)
	require.NoError(t, err)
	require.NotNil(t, validator)
}

func createTrustBaseFromRootGenesis(rootGenesis *genesis.RootGenesis) (types.RootTrustBase, error) {
	var trustBaseNodes []*types.NodeInfo
	var unicityTreeRootHash []byte
	for _, rn := range rootGenesis.Root.RootValidators {
		verifier, err := abcrypto.NewVerifierSecp256k1(rn.SigningPublicKey)
		if err != nil {
			return nil, err
		}
		trustBaseNodes = append(trustBaseNodes, types.NewNodeInfo(rn.NodeIdentifier, 1, verifier))
		// parse unicity tree root hash, optionally sanity check that all root hashes are equal for each partition
		for _, p := range rootGenesis.Partitions {
			if len(unicityTreeRootHash) == 0 {
				unicityTreeRootHash = p.Certificate.UnicitySeal.Hash
			} else if !bytes.Equal(unicityTreeRootHash, p.Certificate.UnicitySeal.Hash) {
				return nil, errors.New("unicity certificate seal hashes are not equal")
			}
		}
	}
	trustBase, err := types.NewTrustBaseGenesis(trustBaseNodes, unicityTreeRootHash)
	if err != nil {
		return nil, err
	}
	return trustBase, nil
}

func TestRootValidatorTest_CertificationReqRejected(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, unknownID, partitionNodes[0])
	require.Error(t, rootValidator.onBlockCertificationRequest(context.Background(), req))
	// unknown id, gets rejected
	require.NotContains(t, rootValidator.incomingRequests.store, unknownID)
	// unknown node gets rejected
	unknownNode := testutils.NewTestNode(t)
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, unknownNode)
	require.ErrorContains(t, rootValidator.onBlockCertificationRequest(context.Background(), req), fmt.Sprintf("node %s is not part of partition trustbase", unknownNode.PeerConf.ID))
	require.NotContains(t, rootValidator.incomingRequests.store, partitionID)
	// signature does not verify
	invalidNode := testutils.TestNode{
		PeerConf: partitionNodes[0].PeerConf,
		Signer:   unknownNode.Signer,
		Verifier: unknownNode.Verifier,
	}
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, &invalidNode)
	require.ErrorContains(t, rootValidator.onBlockCertificationRequest(context.Background(), req), "rejected: signature verification failed")
	require.NotContains(t, rootValidator.incomingRequests.store, partitionID)
}

func TestRootValidatorTest_CertificationReqEquivocatingReq(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	require.NoError(t, rootValidator.onBlockCertificationRequest(context.Background(), req))
	// request is accepted
	key := partitionShard{partition: partitionID, shard: req.Shard.Key()}
	require.Contains(t, rootValidator.incomingRequests.store, key)
	equivocatingIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	eqReq := testutils.CreateBlockCertificationRequest(t, equivocatingIR, partitionID, partitionNodes[0])
	require.ErrorContains(t, rootValidator.onBlockCertificationRequest(context.Background(), eqReq), "request of the node in this round already stored")
	buffer, f := rootValidator.incomingRequests.store[key]
	require.True(t, f, "no requests for %#v", key)
	require.Contains(t, buffer.nodeRequest, partitionNodes[0].PeerConf.ID.String())
	require.Len(t, buffer.requests, 1)
	for _, certReq := range buffer.requests {
		require.Len(t, certReq, 1)
	}
}

func TestRootValidatorTest_SimulateNetCommunication(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create certification request
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].PeerConf.ID, network.ProtocolBlockCertification, req)
	// since consensus is simple majority, then consensus is now achieved and message should be forwarded
	mcm := rootValidator.consensusManager.(*MockConsensusManager)
	require.Eventually(t, func() bool { return len(mcm.certReqCh) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationNoQuorum(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create certification request
	newIR1 := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req1 := testutils.CreateBlockCertificationRequest(t, newIR1, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req1)
	newIR2 := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req2 := testutils.CreateBlockCertificationRequest(t, newIR2, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].PeerConf.ID, network.ProtocolBlockCertification, req2)
	newIR3 := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req3 := testutils.CreateBlockCertificationRequest(t, newIR3, partitionID, partitionNodes[2])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[2].PeerConf.ID, network.ProtocolBlockCertification, req3)
	// no consensus can be achieved all reported different hashes
	mcm := rootValidator.consensusManager.(*MockConsensusManager)
	require.Eventually(t, func() bool { return len(mcm.certReqCh) == 1 }, 1*time.Second, 10*time.Millisecond)
}

func TestRootValidatorTest_SimulateNetCommunicationHandshake(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create
	h := &handshake.Handshake{
		Partition:      partitionID,
		Shard:          types.ShardID{},
		NodeIdentifier: partitionNodes[1].PeerConf.ID.String(),
	}
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolHandshake, h)
	// make sure certificate is sent in return
	testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	// make sure that the node is subscribed
	subscribed := rootValidator.subscription.Get(partitionID)
	require.Empty(t, subscribed)
	// Issue a block certification request -> subscribes
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req)
	subscribed = rootValidator.subscription.Get(partitionID)
	require.Contains(t, subscribed, partitionNodes[0].PeerConf.ID.String())

	// set network in error state
	mockNet.SetErrorState(fmt.Errorf("failed to dial"))
	cr := certification.CertificationResponse{
		Partition: partitionID,
		UC: types.UnicityCertificate{
			InputRecord: newIR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier: partitionID,
			},
			UnicitySeal: &types.UnicitySeal{Version: 1},
		},
	}
	rootValidator.onCertificationResult(ctx, &cr)
	subscribed = rootValidator.subscription.Get(partitionID)
	require.Contains(t, subscribed, partitionNodes[0].PeerConf.ID.String())
	rootValidator.onCertificationResult(ctx, &cr)
	// subscription is cleared, node got two responses and is required to issue a block certification request
	require.Empty(t, rootValidator.subscription.Get(partitionID))
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidReqRoundNumber(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootValidator)
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidHash(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.NotNil(t, rootValidator)
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: test.RandomBytes(32),
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateResponse(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.PeerConf.ID.String())
	// create certification request
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	cr := certification.CertificationResponse{
		Partition: partitionID,
		UC: types.UnicityCertificate{
			InputRecord: newIR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier: partitionID,
			},
			UnicitySeal: &types.UnicitySeal{Version: 1},
		},
	}
	// simulate 2x subscriptions
	id32 := rg.Partitions[0].PartitionDescription.SystemIdentifier
	rootValidator.subscription.Subscribe(id32, rg.Partitions[0].Nodes[0].NodeIdentifier)
	rootValidator.subscription.Subscribe(id32, rg.Partitions[0].Nodes[1].NodeIdentifier)
	// simulate response from consensus manager
	rootValidator.onCertificationResult(ctx, &cr)
	// UC's are sent to all partition nodes
	certs := testutils.MockNetAwaitMultiple[*types.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates, 2)
	require.Len(t, certs, 2)
	for _, cert := range certs {
		require.Equal(t, partitionID, cert.UnicityTreeCertificate.SystemIdentifier)
		require.Equal(t, newIR, cert.InputRecord)
	}
}

func TestRootValidator_ResultUnknown(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
	rootValidator, _, _, rg := initRootValidator(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.Cleanup(ctxCancel)
	go func() { require.ErrorIs(t, rootValidator.Run(ctx), context.Canceled) }()

	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	cr := certification.CertificationResponse{
		Partition: partitionID,
		UC: types.UnicityCertificate{
			InputRecord: newIR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier: unknownID,
			},
			UnicitySeal: &types.UnicitySeal{Version: 1},
		},
	}
	// simulate response from consensus manager
	rootValidator.onCertificationResult(ctx, &cr)
	// no responses will be sent
	require.Empty(t, mockNet.SentMessages(network.ProtocolUnicityCertificates))
}

func TestRootValidator_ExitWhenPendingCertRequestAndCMClosed(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork(t)
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
	newIR := &types.InputRecord{Version: 1,
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].PeerConf.ID, network.ProtocolBlockCertification, req)
	// send second
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].PeerConf.ID, network.ProtocolBlockCertification, req)
	// consensus is achieved and request will sent to CM, but CM is not running
	// node should still exit normally even if CM loop is not running and reading the channel
}
