package rootvalidator

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	"github.com/stretchr/testify/require"
)

var partitionID = []byte{0, 0xFF, 0, 1}
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

func MockConsensus() ConsensusFn {
	return func(partitionStore PartitionStoreRd, stateStore StateStore) (ConsensusManager, error) {
		cm := &MockConsensusManager{
			certReqCh:    make(chan consensus.IRChangeRequest, 1),
			certResultCh: make(chan certificates.UnicityCertificate, 1),
		}
		return cm, nil
	}
}

func initRootValidator(t *testing.T, net PartitionNet) (*Validator, *testutils.TestNode, []*testutils.TestNode, *genesis.RootGenesis) {
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	node := testutils.NewTestNode(t)
	verifier := node.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := node.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), node.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)

	validator, err := NewRootValidatorNode(rootGenesis, node.Peer, net, MockConsensus())
	require.NoError(t, err)
	require.NotNil(t, validator)
	return validator, node, partitionNodes, rootGenesis
}

func TestRootValidatorTest_SimulateNetCommunication(t *testing.T) {
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
