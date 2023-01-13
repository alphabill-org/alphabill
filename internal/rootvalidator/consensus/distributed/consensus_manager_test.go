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
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
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

func TestIRChangeRequestFromRootValidator_RootTimeoutOnFirtstRound(t *testing.T) {
	var lastProposalMsg *atomic_broadcast.ProposalMsg = nil
	var lastVoteMsg *atomic_broadcast.VoteMsg = nil
	var lastTimeoutMsg *atomic_broadcast.TimeoutMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, _, _ := initConsensusManager(t, mockNet)
	defer cm.Stop()
	// Await proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// Quick hack to trigger timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(3), lastTimeoutMsg.Timeout.Round)
	// simulate TC not achieved and make sure the same timeout message is sent again
	// Quick hack to trigger next timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(3), lastTimeoutMsg.Timeout.Round)
	// route timeout message back
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// This triggers TC and next round, wait for proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(4), lastProposalMsg.Block.Round)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(3), lastProposalMsg.LastRoundTc.Timeout.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	// round 3 is skipped, as it timeouts
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.Nil(t, lastVoteMsg.LedgerCommitInfo.RootHash)
}

func TestIRChangeRequestFromRootValidator_RootTimeout(t *testing.T) {
	var lastProposalMsg *atomic_broadcast.ProposalMsg = nil
	var lastVoteMsg *atomic_broadcast.VoteMsg = nil
	var lastTimeoutMsg *atomic_broadcast.TimeoutMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	// generate mock requests from partitions
	requests := make([]*certification.BlockCertificationRequest, 2)
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	irChReq := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: partitionID,
		CertReason:       atomic_broadcast.IRChangeReqMsg_QUORUM,
		Requests:         requests}

	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootIrChangeReq, irChReq)
	// As the node is the leader, next round will trigger a proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Equal(t, partitionID, lastProposalMsg.Block.Payload.Requests[0].SystemIdentifier)
	require.Equal(t, atomic_broadcast.IRChangeReqMsg_QUORUM, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)

	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// route the proposal back to trigger new vote
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)

	// Do not route the vote back, instead simulate round/view timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(4), lastTimeoutMsg.Timeout.Round)
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// this will immediately trigger timeout certificate for the round
	// the following must be true now:
	// round is advanced
	require.Equal(t, uint64(5), cm.pacemaker.currentRound)
	// round pipeline is cleared
	require.Empty(t, cm.roundPipeline.statePipeline)
	// await the next proposal as well, the proposal must contain TC
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	// but it is empty since we did not send new changes
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
}

func TestIRChangeRequestFromRootValidator(t *testing.T) {
	var lastProposalMsg *atomic_broadcast.ProposalMsg = nil
	var lastVoteMsg *atomic_broadcast.VoteMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	// generate mock requests from partitions
	requests := make([]*certification.BlockCertificationRequest, 2)
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	irChReq := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: partitionID,
		CertReason:       atomic_broadcast.IRChangeReqMsg_QUORUM,
		Requests:         requests}

	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootIrChangeReq, irChReq)
	// As the node is the leader, next round will trigger a proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Equal(t, partitionID, lastProposalMsg.Block.Payload.Requests[0].SystemIdentifier)
	require.Equal(t, atomic_broadcast.IRChangeReqMsg_QUORUM, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)

	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// route the proposal back to trigger new vote
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)
	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)

	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// after two successful rounds the IR change will be committed and UC is returned
	require.Eventually(t, func() bool { return len(cm.CertificationResult()) == 1 }, 1*time.Second, 10*time.Millisecond)
}
