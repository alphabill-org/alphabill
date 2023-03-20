package distributed

import (
	"bytes"
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partitions"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

var partitionID = []byte{0, 0xFF, 0, 1}
var partitionInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func readResult(ch <-chan certificates.UnicityCertificate, timeout time.Duration) (*certificates.UnicityCertificate, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("failed to read from channel")
		}
		return &result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
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
	partitions, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	cm, err := NewDistributedAbConsensusManager(rootNode.Peer, rootGenesis, partitions, net, rootNode.Signer)
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
		RoundNumber:  2,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	req := consensus.IRChangeRequest{
		SystemIdentifier: p.SystemIdentifier(partitionID),
		Reason:           consensus.Quorum,
		Requests:         requests}
	cm.RequestCertification() <- req
	// since there is only one root node, it is the next leader, the request will be buffered
	require.Eventually(t, func() bool {
		if cm.irReqBuffer.IsChangeInBuffer(p.SystemIdentifier(partitionID)) == true {
			return true
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func TestIRChangeRequestFromRootValidator_RootTimeoutOnFirstRound(t *testing.T) {
	var lastProposalMsg *atomic_broadcast.ProposalMsg = nil
	var lastVoteMsg *atomic_broadcast.VoteMsg = nil
	var lastTimeoutMsg *atomic_broadcast.TimeoutMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, _, _ := initConsensusManager(t, mockNet)
	defer cm.Stop()
	// Await proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Equal(t, uint64(2), lastProposalMsg.Block.Round)
	// Quick hack to trigger timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// simulate TC not achieved and make sure the same timeout message is sent again
	// Quick hack to trigger next timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// route timeout message back
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// This triggers TC and next round, wait for proposal
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(3), lastProposalMsg.Block.Round)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(2), lastProposalMsg.LastRoundTc.Timeout.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	// round 3 is skipped, as it timeouts
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
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
		RoundNumber:  2,
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
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
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
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)

	// Do not route the vote back, instead simulate round/view timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*atomic_broadcast.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(3), lastTimeoutMsg.Timeout.Round)
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// this will immediately trigger timeout certificate for the round
	// the following must be true now:
	// round is advanced
	require.Equal(t, uint64(4), cm.pacemaker.currentRound)
	// only changes from round 3 are removed, rest will still be active
	require.True(t, cm.blockStore.IsChangeInProgress(p.SystemIdentifier(partitionID)))
	// await the next proposal as well, the proposal must contain TC
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(3), lastProposalMsg.LastRoundTc.Timeout.Round)
	// query state
	getStateMsg := &atomic_broadcast.GetStateMsg{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// no change requests added, previous changes still not committed as timeout occurred
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(4), lastProposalMsg.Block.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// await vote
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	require.Nil(t, lastVoteMsg.LedgerCommitInfo.RootHash)
	// Check state before routing vote back to root
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	stateMsg := testutils.MockAwaitMessage[*atomic_broadcast.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// commit head is still at round 1, as round 3 that would have committed 2 resulted in timeout
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 2, len(stateMsg.BlockNode))
	// round 3 has been removed as it resulted in timeout quorum
	require.Equal(t, uint64(2), stateMsg.BlockNode[0].Block.Round)
	require.Equal(t, uint64(4), stateMsg.BlockNode[1].Block.Round)
	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)

	// await proposal again
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Nil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(5), lastProposalMsg.Block.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)

	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(5), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)
	// Check state before routing vote back to root
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	stateMsg = testutils.MockAwaitMessage[*atomic_broadcast.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// commit head is still at round 1, rounds 2, 4 and 5 are added, 5 will commit 4 when it reaches quorum, but
	// this will after vote is routed back, so current expected state is:
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 3, len(stateMsg.BlockNode))
	// round 3 has been removed as it resulted in timeout quorum
	require.Equal(t, uint64(2), stateMsg.BlockNode[0].Block.Round)
	require.Equal(t, uint64(4), stateMsg.BlockNode[1].Block.Round)
	require.Equal(t, uint64(5), stateMsg.BlockNode[2].Block.Round)

	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// after two successful rounds since timeout the IR change will be finally committed and UC is returned
	result, err := readResult(cm.CertificationResult(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, partitionID, result.UnicityTreeCertificate.SystemIdentifier)
	require.False(t, cm.blockStore.IsChangeInProgress(p.SystemIdentifier(partitionID)))
	// verify certificates have been updated when recovery query is sent
	getCertsMsg := &atomic_broadcast.GetCertificates{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootCertReq, getCertsMsg)
	// As the node is the leader, next round will trigger a proposal
	certsMsg := testutils.MockAwaitMessage[*atomic_broadcast.CertificatesMsg](t, mockNet, network.ProtocolRootCertResp)
	require.Equal(t, len(rg.Partitions), len(certsMsg.Certificates))
	idx := slices.IndexFunc(certsMsg.Certificates, func(c *certificates.UnicityCertificate) bool {
		return bytes.Equal(c.UnicityTreeCertificate.SystemIdentifier, partitionID)
	})
	require.False(t, idx == -1)
	require.True(t, certsMsg.Certificates[idx].UnicitySeal.RootRoundInfo.RoundNumber > uint64(1))
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	// As the node is the leader, next round will trigger a proposal
	stateMsg = testutils.MockAwaitMessage[*atomic_broadcast.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// at this stage the committed round is 4 and round 5 block is pending, if it reaches quorum it will commit 4
	require.Equal(t, uint64(4), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 1, len(stateMsg.BlockNode))
	require.Equal(t, uint64(5), stateMsg.BlockNode[0].Block.Round)
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
		RoundNumber:  2,
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
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
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
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.RootHash)
	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// after two successful rounds the IR change will be committed and UC is returned
	result, err := readResult(cm.CertificationResult(), time.Second)
	trustBase := map[string]crypto.Verifier{rootNode.Peer.ID().String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.IsValid(trustBase, gocrypto.SHA256, partitionID, sdrh))

	// roor will continue and next proposal is also triggered by the same QC
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.NoError(t, err)
}

func TestPartitionTimeoutFromRootValidator(t *testing.T) {
	var lastProposalMsg *atomic_broadcast.ProposalMsg = nil
	var lastVoteMsg *atomic_broadcast.VoteMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, _, _ := initConsensusManager(t, mockNet)
	defer cm.Stop()
	// proposal round 2
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Empty(t, lastProposalMsg.Block.Payload.Requests)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)

	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)

	// proposal round 3
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Empty(t, lastProposalMsg.Block.Payload.Requests)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)

	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)

	// proposal round 4 with timeout
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotEmpty(t, lastProposalMsg.Block.Payload.Requests)
	require.Equal(t, atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// new proposal round 5
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Empty(t, lastProposalMsg.Block.Payload.Requests)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// voting round 5
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(5), lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// triggers timeout certificates for in round 4 to be committed
	result, err := readResult(cm.CertificationResult(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, uint64(5), result.UnicitySeal.RootRoundInfo.RoundNumber)
	// proposal in round 6 should be empty again
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// vote round 6
	lastVoteMsg = testutils.MockAwaitMessage[*atomic_broadcast.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(6), lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// proposal round 7
	lastProposalMsg = testutils.MockAwaitMessage[*atomic_broadcast.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
}

func TestGetCertificates(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	defer cm.Stop()
	getCertsMsg := &atomic_broadcast.GetCertificates{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootCertReq, getCertsMsg)
	// As the node is the leader, next round will trigger a proposal
	certsMsg := testutils.MockAwaitMessage[*atomic_broadcast.CertificatesMsg](t, mockNet, network.ProtocolRootCertResp)
	require.Equal(t, len(rg.Partitions), len(certsMsg.Certificates))
}

func TestGetState(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, _ := initConsensusManager(t, mockNet)
	defer cm.Stop()
	getStateMsg := &atomic_broadcast.GetStateMsg{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	// As the node is the leader, next round will trigger a proposal
	stateMsg := testutils.MockAwaitMessage[*atomic_broadcast.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// at this stage there is only genesis block
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 0, len(stateMsg.BlockNode))
}
