package abdrc

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/rootchain/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	"github.com/alphabill-org/alphabill/internal/types"
)

var partitionID = types.SystemID{0, 0xFF, 0, 1}
var partitionInputRecord = &types.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func readResult(ch <-chan *types.UnicityCertificate, timeout time.Duration) (*types.UnicityCertificate, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("failed to read from channel")
		}
		return result, nil
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
	require.NoError(t, err)
	cm, err := NewDistributedAbConsensusManager(id, rootGenesis, partitions, net, rootNode.Signer)
	require.NoError(t, err)
	return cm, rootNode, partitionNodes, rootGenesis
}

func buildBlockCertificationRequest(t *testing.T, rg *genesis.RootGenesis, partitionNodes []*testutils.TestNode) []*certification.BlockCertificationRequest {
	t.Helper()
	newIR := &types.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  2,
	}
	requests := make([]*certification.BlockCertificationRequest, len(partitionNodes))
	for i, n := range partitionNodes {
		requests[i] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, n)
	}
	return requests
}

func TestNewConsensusManager_Ok(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, root, partitionNodes, rg := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	require.Len(t, partitionNodes, 3)
	require.NotNil(t, cm)
	require.NotNil(t, root)
	require.NotNil(t, rg)
}

func Test_ConsensusManager_onPartitionIRChangeReq(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, _, partitionNodes, rg := initConsensusManager(t, mockNet)

	req := &consensus.IRChangeRequest{
		SystemIdentifier: partitionID,
		Reason:           consensus.Quorum,
		Requests:         buildBlockCertificationRequest(t, rg, partitionNodes),
	}

	// we need to init pacemaker into correct round, otherwise IR validation fails
	cm.pacemaker.Reset(cm.blockStore.GetHighQc().VoteInfo.RoundNumber)
	defer cm.pacemaker.Stop()

	require.NoError(t, cm.onPartitionIRChangeReq(req))
	// since there is only one root node, it is the next leader, the request will be buffered
	require.True(t, cm.irReqBuffer.IsChangeInBuffer(p.SystemIdentifier(partitionID)))
}

func TestIRChangeRequestFromRootValidator_RootTimeoutOnFirstRound(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil
	var lastTimeoutMsg *abdrc.TimeoutMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, _, _ := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// Await proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Equal(t, uint64(2), lastProposalMsg.Block.Round)
	// Quick hack to trigger timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// simulate TC not achieved and make sure the same timeout message is sent again
	// Quick hack to trigger next timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	lastTimeoutMsg = testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// route timeout message back
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// This triggers TC and next round, wait for proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(3), lastProposalMsg.Block.Round)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(2), lastProposalMsg.LastRoundTc.Timeout.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	// round 3 is skipped, as it timeouts
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.Nil(t, lastVoteMsg.LedgerCommitInfo.Hash)
}

func TestIRChangeRequestFromRootValidator_RootTimeout(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil
	var lastTimeoutMsg *abdrc.TimeoutMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// simulate IR change request message
	irChReq := &abtypes.IRChangeReq{
		SystemIdentifier: partitionID,
		CertReason:       abtypes.Quorum,
		Requests:         buildBlockCertificationRequest(t, rg, partitionNodes[0:2]),
	}
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootIrChangeReq, irChReq)
	// As the node is the leader, next round will trigger a proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, bytes.Equal(partitionID, lastProposalMsg.Block.Payload.Requests[0].SystemIdentifier))
	require.Equal(t, abtypes.Quorum, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)

	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// route the proposal back to trigger new vote
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)

	// Do not route the vote back, instead simulate round/view timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout()
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(3), lastTimeoutMsg.Timeout.Round)
	// route the timeout message back to trigger timeout certificate and new round
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootTimeout, lastTimeoutMsg)
	// this will immediately trigger timeout certificate for the round
	// the following must be true now:
	// round is advanced
	require.Equal(t, uint64(4), cm.pacemaker.GetCurrentRound())
	// only changes from round 3 are removed, rest will still be active
	require.True(t, cm.blockStore.IsChangeInProgress(p.SystemIdentifier(partitionID)))
	// await the next proposal as well, the proposal must contain TC
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(3), lastProposalMsg.LastRoundTc.Timeout.Round)
	// query state
	getStateMsg := &abdrc.GetStateMsg{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// no change requests added, previous changes still not committed as timeout occurred
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(4), lastProposalMsg.Block.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// await vote
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(4), lastVoteMsg.VoteInfo.RoundNumber)
	require.Nil(t, lastVoteMsg.LedgerCommitInfo.Hash)
	// Check state before routing vote back to root
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	stateMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// commit head is still at round 1, as round 3 that would have committed 2 resulted in timeout
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 2, len(stateMsg.BlockNode))
	// round 3 has been removed as it resulted in timeout quorum
	require.Equal(t, uint64(2), stateMsg.BlockNode[0].Block.Round)
	require.Equal(t, uint64(4), stateMsg.BlockNode[1].Block.Round)
	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)

	// await proposal again
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Nil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(5), lastProposalMsg.Block.Round)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)

	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(5), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)
	// Check state before routing vote back to root
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	stateMsg = testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
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
	getCertsMsg := &abdrc.GetStateMsg{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getCertsMsg)
	// As the node is the leader, next round will trigger a proposal
	certsMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	require.Equal(t, len(rg.Partitions), len(certsMsg.Certificates))
	idx := slices.IndexFunc(certsMsg.Certificates, func(c *types.UnicityCertificate) bool {
		return bytes.Equal(c.UnicityTreeCertificate.SystemIdentifier, partitionID)
	})
	require.False(t, idx == -1)
	require.True(t, certsMsg.Certificates[idx].UnicitySeal.RootChainRoundNumber > uint64(1))
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	// As the node is the leader, next round will trigger a proposal
	stateMsg = testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// at this stage the committed round is 4 and round 5 block is pending, if it reaches quorum it will commit 4
	require.Equal(t, uint64(4), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 1, len(stateMsg.BlockNode))
	require.Equal(t, uint64(5), stateMsg.BlockNode[0].Block.Round)
}

func TestIRChangeRequestFromRootValidator(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// simulate IR change request message
	irChReq := &abtypes.IRChangeReq{
		SystemIdentifier: partitionID,
		CertReason:       abtypes.Quorum,
		Requests:         buildBlockCertificationRequest(t, rg, partitionNodes[0:2]),
	}
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootIrChangeReq, irChReq)
	// As the node is the leader, next round will trigger a proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, bytes.Equal(partitionID, lastProposalMsg.Block.Payload.Requests[0].SystemIdentifier))
	require.Equal(t, abtypes.Quorum, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)
	// route the proposal back
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)

	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// route the proposal back to trigger new vote
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)
	// send vote back to validator
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// after two successful rounds the IR change will be committed and UC is returned
	result, err := readResult(cm.CertificationResult(), time.Second)
	trustBase := map[string]crypto.Verifier{rootNode.Peer.ID().String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.IsValid(trustBase, gocrypto.SHA256, partitionID, sdrh))

	// roor will continue and next proposal is also triggered by the same QC
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.NoError(t, err)
}

func TestPartitionTimeoutFromRootValidator(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil

	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, _, rg := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	roundNo := uint64(1) // 1 is genesis
	// run a loop of 11 rounds to produce a root chain timeout
	for i := 0; i < int(rg.Partitions[0].SystemDescriptionRecord.T2Timeout/(rg.Root.Consensus.BlockRateMs/2)); i++ {
		// proposal rounds 2..
		roundNo++
		lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
		require.Empty(t, lastProposalMsg.Block.Payload.Requests)
		// route the proposal back
		testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
		// wait for the vote message
		lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
		require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
		testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	}
	// proposal round 7 with timeout
	roundNo++
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotEmpty(t, lastProposalMsg.Block.Payload.Requests)
	require.Equal(t, abtypes.T2Timeout, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// new proposal round 8
	roundNo++
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Empty(t, lastProposalMsg.Block.Payload.Requests)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// voting round 8
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// triggers timeout certificates for in round 8 to be committed
	result, err := readResult(cm.CertificationResult(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	// round 7 got certified in round 8
	require.Equal(t, roundNo-1, result.UnicitySeal.RootChainRoundNumber)
	// proposal in round 9 should be empty again
	roundNo++
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
	// vote round 9
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootVote, lastVoteMsg)
	// proposal round 10
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootProposal, lastProposalMsg)
}

func TestGetState(t *testing.T) {
	mockNet := testnetwork.NewMockNetwork()
	cm, rootNode, partitionNodes, _ := initConsensusManager(t, mockNet)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	getStateMsg := &abdrc.GetStateMsg{
		NodeId: partitionNodes[0].Peer.ID().String(),
	}
	// simulate IR change request message
	testutils.MockValidatorNetReceives(t, mockNet, rootNode.Peer.ID(), network.ProtocolRootStateReq, getStateMsg)
	// As the node is the leader, next round will trigger a proposal
	stateMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// at this stage there is only genesis block
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 0, len(stateMsg.BlockNode))
	require.Len(t, stateMsg.Certificates, 1)
}

func Test_ConsensusManager_messages(t *testing.T) {
	t.Parallel()

	// partition data used/shared by tests
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 2)

	t.Run("IR change request from partition included in proposal", func(t *testing.T) {
		cms, rootNet, rootG := createConsensusManagers(t, 1, []*genesis.PartitionRecord{partitionRecord})

		// proposal will be broadcasted so eavesdrop the network and make copy of it
		propCh := make(chan *abdrc.ProposalMsg, 1)
		rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
			if msg, ok := msg.Message.(*abdrc.ProposalMsg); ok {
				propCh <- msg
			}
			return false
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cms[0].Run(ctx), context.Canceled) }()

		// simulate root validator node sending IRCR to consensus manager
		irCReq := consensus.IRChangeRequest{
			SystemIdentifier: partitionID,
			Reason:           consensus.Quorum,
			Requests:         buildBlockCertificationRequest(t, rootG, partitionNodes),
		}

		select {
		case <-time.After(cms[0].pacemaker.minRoundLen):
			t.Fatal("CM doesn't consume IR change request")
		case cms[0].RequestCertification() <- irCReq:
		}

		// IRCR must be included into proposal
		select {
		case <-time.After(cms[0].pacemaker.maxRoundLen):
			t.Fatal("haven't got the proposal before timeout")
		case prop := <-propCh:
			require.NotNil(t, prop)
			require.NotNil(t, prop.Block)
			require.NotNil(t, prop.Block.Payload)
			require.Len(t, prop.Block.Payload.Requests, 1)
			require.EqualValues(t, irCReq.SystemIdentifier, prop.Block.Payload.Requests[0].SystemIdentifier)
			require.ElementsMatch(t, irCReq.Requests, prop.Block.Payload.Requests[0].Requests)
		}
	})

	t.Run("IR change request forwarded by peer included in proposal", func(t *testing.T) {
		cms, rootNet, rootG := createConsensusManagers(t, 1, []*genesis.PartitionRecord{partitionRecord})
		cmLeader := cms[0]

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cmLeader.Run(ctx), context.Canceled) }()

		// simulate "other root node" forwarding IRCR to leader
		irCReq := &abtypes.IRChangeReq{
			SystemIdentifier: partitionID,
			CertReason:       abtypes.Quorum,
			Requests:         buildBlockCertificationRequest(t, rootG, partitionNodes),
		}

		cmBid, _, _, _ := generatePeerData(t)
		cmBnet := rootNet.Connect(cmBid)
		msg := network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irCReq,
		}
		require.NoError(t, cmBnet.Send(msg, []peer.ID{cmLeader.id}))

		// IRCR must be included into broadcasted proposal, either this or next round
		sawIRCR := false
		for cnt := 0; cnt < 2 && !sawIRCR; cnt++ {
			select {
			case <-time.After(cmLeader.pacemaker.maxRoundLen):
				t.Fatal("haven't got the proposal before timeout")
			case msg := <-cmBnet.ReceivedChannel():
				prop := msg.Message.(*abdrc.ProposalMsg)
				require.NotNil(t, prop)
				require.NotNil(t, prop.Block)
				require.NotNil(t, prop.Block.Payload)
				if len(prop.Block.Payload.Requests) == 1 {
					require.EqualValues(t, irCReq.SystemIdentifier, prop.Block.Payload.Requests[0].SystemIdentifier)
					require.ElementsMatch(t, irCReq.Requests, prop.Block.Payload.Requests[0].Requests)
					sawIRCR = true
				}
			}
		}
		require.True(t, sawIRCR, "expected to see the IRCR in one of the next two proposals")
	})

	t.Run("state request triggers response", func(t *testing.T) {
		cms, _, _ := createConsensusManagers(t, 2, []*genesis.PartitionRecord{partitionRecord})
		cmA, cmB := cms[0], cms[1]

		// only launch cmA, we manage cmB "manually"
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cmA.Run(ctx), context.Canceled) }()

		// cmB sends state request to cmA
		msg := network.OutputMessage{
			Protocol: network.ProtocolRootStateReq,
			Message:  &abdrc.GetStateMsg{NodeId: cmB.id.String()},
		}
		require.NoError(t, cmB.net.Send(msg, []peer.ID{cmA.id}))

		// cmB should receive state response
		select {
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("timeout while waiting for recovery response")
		case msg := <-cmB.net.ReceivedChannel():
			state := msg.Message.(*abdrc.StateMsg)
			require.NotNil(t, state)
			require.EqualValues(t, 1, state.CommittedHead.Block.Round)
			require.Empty(t, state.BlockNode)
			require.Len(t, state.Certificates, 1)
		}
	})
}

func Test_selectRandomNodeIdsFromSignatureMap(t *testing.T) {
	t.Parallel()

	// generate some valid peer IDs for tests to use
	peerIDs := make(peer.IDSlice, 3)
	for i := range peerIDs {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		peerIDs[i], err = network.NodeIDFromPublicKeyBytes(pubKeyBytes)
		require.NoError(t, err)
	}
	idA, idB, idC := peerIDs[0], peerIDs[1], peerIDs[2]

	t.Run("empty inputs", func(t *testing.T) {
		nodes := selectRandomNodeIdsFromSignatureMap(nil, 2)
		require.Empty(t, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string][]byte{}, 2)
		require.Empty(t, nodes)
	})

	t.Run("no duplicates added", func(t *testing.T) {
		nodes := selectRandomNodeIdsFromSignatureMap(map[string][]byte{idA.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idA}, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string][]byte{idA.String(): nil, idB.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idA, idB}, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string][]byte{idA.String(): nil, idB.String(): nil}, 3)
		require.ElementsMatch(t, []peer.ID{idA, idB}, nodes)
	})

	t.Run("invalid IDs are ignored", func(t *testing.T) {
		nodes := selectRandomNodeIdsFromSignatureMap(map[string][]byte{"foo bar": nil}, 1)
		require.Empty(t, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string][]byte{"foo bar": nil, idB.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idB}, nodes)
	})

	t.Run("max count items is returned", func(t *testing.T) {
		inp := map[string][]byte{idA.String(): nil, idB.String(): nil, idC.String(): nil}

		nodes := selectRandomNodeIdsFromSignatureMap(inp, 1)
		require.Len(t, nodes, 1)

		nodes = selectRandomNodeIdsFromSignatureMap(inp, 2)
		require.Len(t, nodes, 2)
		require.NotEqual(t, nodes[0], nodes[1])
	})
}

func Test_rootNetworkRunning(t *testing.T) {
	t.Parallel()
	// this test is mostly useful for debugging - modify conditions in the test,
	// launch the test and observe logs...

	// for quorum we need ⅔+1 validators (root nodes) to be healthy
	const rootNodeCnt = 4
	// destination round - until which round (minimum) the test should run. test stops as soon as
	// one node is in that round so last round might be "incomplete". System starts with round 2.
	const destRound = 10

	// consumeUC acts as a validator node consuming the UC-s generated by CM (until ctx is cancelled).
	// UC-s need to be consumed as otherwise CM blocks where it tries to send them, ie T2 timeout!
	consumeUC := func(ctx context.Context, cm *ConsensusManager) {
		for {
			select {
			case <-ctx.Done():
				return
			case uc := <-cm.certResultCh:
				t.Logf("%s UC for round %d sent to validator", cm.id.ShortString(), uc.GetRoundNumber())
			}
		}
	}

	partitionRecs := []*genesis.PartitionRecord{
		createPartitionRecord(t, partitionID, partitionInputRecord, 1),
	}

	cms, rootNet, _ := createConsensusManagers(t, rootNodeCnt, partitionRecs)

	var totalMsgCnt atomic.Uint32
	rootNet.SetFirewall(func(from, to peer.ID, msg network.OutputMessage) bool {
		msgCnt := totalMsgCnt.Add(1)
		block := msgCnt%200 == 0 // drop every n-th message from the network
		t.Logf("%t # %s -> %s : %T", block, from.ShortString(), to.ShortString(), msg.Message)
		return block
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	for _, v := range cms {
		go func(cm *ConsensusManager) { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }(v)
		go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
	}

	cm := cms[0]
	// assume rounds are successful and each round takes between minRoundLen and roundTimeout on average
	maxTestDuration := destRound * (cm.pacemaker.minRoundLen + (cm.pacemaker.maxRoundLen-cm.pacemaker.minRoundLen)/2)
	require.Eventually(t, func() bool { return cm.pacemaker.GetCurrentRound() >= destRound }, maxTestDuration, 100*time.Millisecond, "waiting for round %d to be achieved", destRound)
	stop := time.Now()
	cancel()
	// when calculating expected message counts keep in mind that last round might not be complete
	// ie the test ended before all nodes had a chance to post their message. so use -1 rounds!
	// and we start from round 2 so thats another -1 completed rounds.
	completeRounds := cm.pacemaker.GetCurrentRound() - 2
	avgRoundLen := time.Duration(int64(stop.Sub(start)) / int64(completeRounds))

	// output some statistics
	t.Logf("total msg count: %d during %s", totalMsgCnt.Load(), stop.Sub(start))

	// check some expectations
	// we expect to see proposal + vote per node per round. when some round timeouts msg count is higher!
	require.GreaterOrEqual(t, totalMsgCnt.Load(), uint32(completeRounds*rootNodeCnt*2), "total number of messages in the network")
	// average round duration should be between minRoundLen and maxRoundLen (aka timeout)
	// potentially flaky as there is delay between starting CMs and starting the clock!
	require.GreaterOrEqual(t, avgRoundLen, cm.pacemaker.minRoundLen, "minimum round duration for %d rounds", completeRounds)
	require.GreaterOrEqual(t, cm.pacemaker.maxRoundLen, avgRoundLen, "maximum round duration for %d rounds", completeRounds)
}
