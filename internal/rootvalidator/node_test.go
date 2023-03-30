package rootvalidator

import (
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/monolithic"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partitions"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	"github.com/stretchr/testify/require"
)

var partitionID = []byte{0, 0xFF, 0, 1}
var unknownID = []byte{0, 0, 0, 0}
var partitionInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

type MockConsensusManager struct {
	certReqCh    chan consensus.IRChangeRequest
	certResultCh chan certificates.UnicityCertificate
	partitions   partitions.PartitionConfiguration
	certs        map[p.SystemIdentifier]*certificates.UnicityCertificate
}

func NewMockConsensus(rg *genesis.RootGenesis, partitionStore partitions.PartitionConfiguration) (*MockConsensusManager, error) {
	var c = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rg.Partitions {
		c[partition.GetSystemIdentifierString()] = partition.Certificate
	}

	return &MockConsensusManager{
		// use buffered channels here, we just want to know if a tlg is received
		certReqCh:    make(chan consensus.IRChangeRequest, 1),
		certResultCh: make(chan certificates.UnicityCertificate, 1),
		partitions:   partitionStore,
		certs:        c,
	}, nil
}

func (m *MockConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return m.certReqCh
}

func (m *MockConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return m.certResultCh
}

func (m *MockConsensusManager) Stop() {
	// nothing to do
}

func (m *MockConsensusManager) GetLatestUnicityCertificate(id p.SystemIdentifier) (*certificates.UnicityCertificate, error) {
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
	cm, err := monolithic.NewMonolithicConsensusManager(node.Peer.ID().String(),
		rootGenesis,
		partitionStore,
		node.Signer)
	require.NoError(t, err)
	validator, err := New(node.Peer, mockNet, partitionStore, cm)
	require.NoError(t, err)
	require.NotNil(t, validator)
	defer validator.Close()
}

func TestRootValidatorTest_CertificationReqRejected(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, unknownID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(req)
	// unknown id, gets rejected
	require.NotContains(t, rootValidator.incomingRequests.store, p.SystemIdentifier(unknownID))
	// unknown node gets rejected
	unknownNode := testutils.NewTestNode(t)
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, unknownNode)
	rootValidator.onBlockCertificationRequest(req)
	require.NotContains(t, rootValidator.incomingRequests.store, p.SystemIdentifier(partitionID))
	// signature does not verify
	invalidNode := testutils.TestNode{
		Peer:     partitionNodes[0].Peer,
		Signer:   unknownNode.Signer,
		Verifier: unknownNode.Verifier,
	}
	req = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, &invalidNode)
	rootValidator.onBlockCertificationRequest(req)
	require.NotContains(t, rootValidator.incomingRequests.store, p.SystemIdentifier(partitionID))
	//
}

func TestRootValidatorTest_CertificationReqEquivocatingReq(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(req)
	// request is accepted
	require.Contains(t, rootValidator.incomingRequests.store, p.SystemIdentifier(partitionID))
	equivocatingIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	eqReq := testutils.CreateBlockCertificationRequest(t, equivocatingIR, partitionID, partitionNodes[0])
	rootValidator.onBlockCertificationRequest(eqReq)
	buffer, f := rootValidator.incomingRequests.store[p.SystemIdentifier(partitionID)]
	require.True(t, f)
	storedNodeReq, f := buffer.requests[partitionNodes[0].Peer.ID().String()]
	require.True(t, f)
	require.Equal(t, req, storedNodeReq)
	require.Len(t, buffer.hashCounts, 1)
	for _, count := range buffer.hashCounts {
		require.Equal(t, uint(1), count)
	}
}

func TestRootValidatorTest_SimulateNetCommunication(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR := &certificates.InputRecord{
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
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR1 := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req1 := testutils.CreateBlockCertificationRequest(t, newIR1, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req1)
	newIR2 := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req2 := testutils.CreateBlockCertificationRequest(t, newIR2, partitionID, partitionNodes[1])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[1].Peer.ID(), network.ProtocolBlockCertification, req2)
	newIR3 := &certificates.InputRecord{
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
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create
	h := &handshake.Handshake{
		SystemIdentifier: partitionID,
		NodeIdentifier:   partitionNodes[1].Peer.ID().String(),
	}
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolHandshake, h)
	// no drama, make sure it is received and does not crash or anything, this message should be removed someday
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidReqRoundNumber(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*certificates.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateNetCommunicationInvalidHash(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	newIR := &certificates.InputRecord{
		PreviousHash: test.RandomBytes(32),
		Hash:         newHash,
		BlockHash:    blockHash,
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	req := testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	testutils.MockValidatorNetReceives(t, mockNet, partitionNodes[0].Peer.ID(), network.ProtocolBlockCertification, req)
	// expect repeat UC to be sent
	repeatCert := testutils.MockAwaitMessage[*certificates.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates)
	require.Equal(t, rg.Partitions[0].Certificate, repeatCert)
}

func TestRootValidatorTest_SimulateResponse(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, node, partitionNodes, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, rg)
	require.NotEmpty(t, node.Peer.ID().String())
	// create certification request
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	uc := &certificates.UnicityCertificate{
		InputRecord: newIR,
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: partitionID,
		},
		UnicitySeal: &certificates.UnicitySeal{},
	}
	// simulate 2x subscriptions
	rootValidator.subscription.Subscribe(p.SystemIdentifier(rg.Partitions[0].SystemDescriptionRecord.SystemIdentifier), rg.Partitions[0].Nodes[0].NodeIdentifier)
	rootValidator.subscription.Subscribe(p.SystemIdentifier(rg.Partitions[0].SystemDescriptionRecord.SystemIdentifier), rg.Partitions[0].Nodes[1].NodeIdentifier)
	// simulate response from consensus manager
	rootValidator.onCertificationResult(uc)
	// UC's are sent to all partition nodes
	certs := testutils.MockNetAwaitMultiple[*certificates.UnicityCertificate](t, mockNet, network.ProtocolUnicityCertificates, 2)
	require.Len(t, certs, 2)
	for _, cert := range certs {
		require.Equal(t, partitionID, cert.UnicityTreeCertificate.SystemIdentifier)
		require.Equal(t, newIR, cert.InputRecord)
	}
}

func TestRootValidator_ResultUnknown(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	rootValidator, _, _, rg := initRootValidator(t, mockNet)
	defer rootValidator.Close()
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	uc := &certificates.UnicityCertificate{
		InputRecord: newIR,
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier: unknownID,
		},
		UnicitySeal: &certificates.UnicitySeal{},
	}
	// simulate response from consensus manager
	rootValidator.onCertificationResult(uc)
	// no responses will be sent
	require.Empty(t, mockNet.SentMessages(network.ProtocolUnicityCertificates))
}
