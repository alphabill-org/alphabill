package distributed

import (
	"bytes"
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
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
}

func initConsensusManager(t *testing.T, net RootNet) (*ConsensusManager, *testutils.TestNode, []*testutils.TestNode, *genesis.RootGenesis) {
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	// initiate state store
	stateStore := store.NewInMemStateStore(gocrypto.SHA256)
	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
	}
	require.NoError(t, stateStore.Save(store.RootState{
		LatestRound:    rootGenesis.GetRoundNumber(),
		Certificates:   certs,
		LatestRootHash: rootGenesis.GetRoundHash(),
	}))
	cm, err := NewDistributedAbConsensusManager(rootNode.Peer, rootGenesis.GetRoot(), partitions, stateStore, rootNode.Signer, net)
	require.NoError(t, err)
	return cm, rootNode, partitionNodes, rootGenesis
}

func TestNewConsensusManager_Ok(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, root, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	require.Len(t, partitionNodes, 3)
	require.NotNil(t, cm)
	require.NotNil(t, root)
	require.NotNil(t, rg)
}

func TestIRChangeRequestFromPartition(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, _, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	requests := make([]*certification.BlockCertificationRequest, 2)
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	requests[0] = testutils.CreateBlockCertificationRequest(t, rg.Partitions[0].Nodes[0], partitionID, newHash, blockHash, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, rg.Partitions[0].Nodes[1], partitionID, newHash, blockHash, partitionNodes[1])
	req := consensus.IRChangeRequest{
		SystemIdentifier: p.SystemIdentifier(partitionID),
		Reason:           consensus.Quorum,
		IR:               requests[0].InputRecord,
		Requests:         requests}
	cm.RequestCertification() <- req
	// verify the IR change request is forwarded to the next leader (self as there is only one)
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages(network.ProtocolRootIrChangeReq)
		if len(messages) > 0 {
			m := messages[0]
			irCh := m.Message.(*atomic_broadcast.IRChangeReqMsg)
			return bytes.Equal(irCh.SystemIdentifier, partitionID) && irCh.CertReason == atomic_broadcast.IRChangeReqMsg_QUORUM &&
				len(irCh.Requests) == 2
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func TestIRChangeRequestFromRootValidator(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	requests := make([]*certification.BlockCertificationRequest, 2)
	newHash := test.RandomBytes(32)
	blockHash := test.RandomBytes(32)
	requests[0] = testutils.CreateBlockCertificationRequest(t, rg.Partitions[0].Nodes[0], partitionID, newHash, blockHash, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, rg.Partitions[0].Nodes[1], partitionID, newHash, blockHash, partitionNodes[1])
	irChReq := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: partitionID,
		CertReason:       atomic_broadcast.IRChangeReqMsg_QUORUM,
		Requests:         requests}
	// simulate IR change request message
	mockNet.Receive(network.ReceivedMessage{
		From:     rootNode.Peer.ID(),
		Protocol: network.ProtocolRootIrChangeReq,
		Message:  irChReq,
	})
	// wait for consensus manager to read the IR change and then trigger timeout
	require.Eventually(t, func() bool { return len(mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// simulate new round event timeout to trigger proposal
	cm.processNewRoundEvent()
	// simulate two consecutive rounds
	// verify the IR change request is forwarded to the next leader (self as there is only one)
	var proposalMsg *atomic_broadcast.ProposalMsg = nil
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages(network.ProtocolRootProposal)
		if len(messages) > 0 {
			m := messages[0]
			proposalMsg = m.Message.(*atomic_broadcast.ProposalMsg)
			if proposalMsg.Block.Payload.IsEmpty() {
				return false
			}
			return bytes.Equal(proposalMsg.Block.Payload.Requests[0].SystemIdentifier, partitionID) && proposalMsg.Block.Payload.Requests[0].CertReason == atomic_broadcast.IRChangeReqMsg_QUORUM &&
				len(proposalMsg.Block.Payload.Requests[0].Requests) == 2
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// route the proposal back
	mockNet.Receive(network.ReceivedMessage{
		From:     rootNode.Peer.ID(),
		Protocol: network.ProtocolRootProposal,
		Message:  proposalMsg,
	})
	require.Eventually(t, func() bool { return len(mockNet.MessageCh) == 0 }, 100000*time.Second, 10*time.Millisecond)
	mockNet.ResetSentMessages(network.ProtocolRootProposal)
	var voteMsg *atomic_broadcast.VoteMsg = nil
	// wait for the vote message
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages(network.ProtocolRootVote)
		if len(messages) > 0 {
			m := messages[0]
			voteMsg = m.Message.(*atomic_broadcast.VoteMsg)
			if proposalMsg.Block.Payload.IsEmpty() {
				return false
			}
			return voteMsg.VoteInfo.RoundNumber == 3 && voteMsg.VoteInfo.ParentRoundNumber == 2 && voteMsg.VoteInfo.Epoch == 0 && voteMsg.LedgerCommitInfo.RootHash != nil
		}
		return false
	}, 100000*test.WaitDuration, test.WaitTick)
	// send vote back to validator
	mockNet.Receive(network.ReceivedMessage{
		From:     rootNode.Peer.ID(),
		Protocol: network.ProtocolRootVote,
		Message:  voteMsg,
	})
	// wait vote processed
	require.Eventually(t, func() bool { return len(mockNet.MessageCh) == 0 }, 10000*time.Second, 10*time.Millisecond)
	mockNet.ResetSentMessages(network.ProtocolRootVote)
	// this will trigger next proposal since QC is achieved
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages(network.ProtocolRootProposal)
		if len(messages) > 0 {
			m := messages[0]
			proposalMsg = m.Message.(*atomic_broadcast.ProposalMsg)
			if proposalMsg.Block.Payload.IsEmpty() {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// route the proposal back
	mockNet.Receive(network.ReceivedMessage{
		From:     rootNode.Peer.ID(),
		Protocol: network.ProtocolRootProposal,
		Message:  proposalMsg,
	})
	require.Eventually(t, func() bool { return len(mockNet.MessageCh) == 0 }, 100000*time.Second, 10*time.Millisecond)
	mockNet.ResetSentMessages(network.ProtocolRootProposal)
	// wait for the vote message
	require.Eventually(t, func() bool {
		messages := mockNet.SentMessages(network.ProtocolRootVote)
		if len(messages) > 0 {
			m := messages[0]
			voteMsg = m.Message.(*atomic_broadcast.VoteMsg)
			if proposalMsg.Block.Payload.IsEmpty() {
				return true
			}
		}
		return false
	}, 100000*test.WaitDuration, test.WaitTick)
	// send vote back to validator
	mockNet.Receive(network.ReceivedMessage{
		From:     rootNode.Peer.ID(),
		Protocol: network.ProtocolRootVote,
		Message:  voteMsg,
	})
	// after two successful rounds the IR change will be committed and UC is returned
	require.Eventually(t, func() bool { return len(cm.CertificationResult()) == 1 }, 10000*time.Second, 10*time.Millisecond)
}
