package consensus

import (
	"context"
	"crypto"
	"crypto/rand"
	"fmt"
	"maps"
	"os"
	"runtime/pprof"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	p2ptest "github.com/libp2p/go-libp2p/core/test"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobservability "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	abdrctu "github.com/alphabill-org/alphabill/rootchain/consensus/testutils"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
)

const partitionID types.PartitionID = 0x00FF0001

var shardID = types.ShardID{}

func readResult(ch <-chan *certification.CertificationResponse, timeout time.Duration) (*types.UnicityCertificate, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("failed to read from channel")
		}
		return &result.UC, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func initConsensusManager(t *testing.T, rootNet RootNet, opts ...Option) (*ConsensusManager, *testutils.TestNode, []*testutils.TestNode) {
	rootNode := testutils.NewTestNode(t)
	shardNodes, shardNodeInfos := testutils.CreateTestNodes(t, 3)
	trustBase := trustbase.NewTrustBaseFromVerifiers(t, map[string]abcrypto.Verifier{
		rootNode.PeerConf.ID.String(): rootNode.Verifier,
	})
	observe := testobservability.Default(t)

	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     partitionID,
		ShardID:         shardID,
		PartitionTypeID: 999,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		Validators:      shardNodeInfos,
		Epoch:           0,
		EpochStart:      1,
	}

	orchestration := testpartition.NewOrchestration(t, observe.Logger())
	require.NoError(t, orchestration.AddShardConfig(shardConf))

	rootSigners := map[string]abcrypto.Signer{
		rootNode.PeerConf.ID.String(): rootNode.Signer,
	}
	opts = append(opts, WithStorage(createStorage(t, shardConf, rootSigners)))

	cm, err := NewConsensusManager(
		rootNode.PeerConf.ID,
		trustBase,
		orchestration,
		rootNet,
		rootNode.Signer,
		observe,
		opts...,
	)
	require.NoError(t, err)

	return cm, rootNode, shardNodes
}

func buildBlockCertificationRequest(t *testing.T, shardNodes []*testutils.TestNode, lastCR *certification.CertificationResponse) []*certification.BlockCertificationRequest {
	t.Helper()
	timestamp := uint64(1)
	if lastCR != nil {
		timestamp = lastCR.UC.UnicitySeal.Timestamp
	}
	newIR := &types.InputRecord{
		Version:      1,
		PreviousHash: nil,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: []byte{2, 3, 4},
		RoundNumber:  1,
		Timestamp:    timestamp,
	}
	requests := make([]*certification.BlockCertificationRequest, len(shardNodes))
	for i, n := range shardNodes {
		requests[i] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, n)
	}
	return requests
}

func Test_ConsensusManager_onPartitionIRChangeReq(t *testing.T) {
	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, shardNodes := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	require.Eventually(t, func() bool {
		return cm.pacemaker.GetCurrentRound() > 0
	}, test.WaitDuration, test.WaitShortTick)

	si, err := cm.ShardInfo(partitionID, shardID)
	require.NoError(t, err)

	req := &IRChangeRequest{
		Partition: partitionID,
		Reason:    Quorum,
		Requests:  buildBlockCertificationRequest(t, shardNodes, si.LastCR),
	}

	require.NoError(t, cm.onPartitionIRChangeReq(context.Background(), req))
	// since there is only one root node, it is the next leader, the request will be buffered
	require.True(t, cm.irReqBuffer.IsChangeInBuffer(partitionID, shardID))
}

func Test_ConsensusManager_onIRChangeMsg_ErrInvalidSignature(t *testing.T) {
	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, shardNodes := initConsensusManager(t, mockNet)

	req := &abdrc.IrChangeReqMsg{
		Author: cm.id.String(),
		IrChangeReq: &drctypes.IRChangeReq{
			Partition:  partitionID,
			CertReason: drctypes.Quorum,
			Requests:   buildBlockCertificationRequest(t, shardNodes, nil),
		},
		Signature: []byte{1, 2, 3, 4},
	}
	// verify that error is printed and author ID is also present
	require.ErrorContains(t, cm.onIRChangeMsg(context.Background(), req),
		fmt.Sprintf("invalid IR change request from node %s: signature verification failed", cm.id.String()))
}

func TestIRChangeRequestFromRootValidator_RootTimeoutOnFirstRound(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil
	var lastTimeoutMsg *abdrc.TimeoutMsg = nil

	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, _ := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// Await proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Equal(t, uint64(2), lastProposalMsg.Block.Round)
	// Quick hack to trigger timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout(ctx)
	// await timeout vote
	lastTimeoutMsg = testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// simulate TC not achieved and make sure the same timeout message is sent again
	// Quick hack to trigger next timeout
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout(ctx)
	lastTimeoutMsg = testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(2), lastTimeoutMsg.Timeout.Round)
	// route timeout message back
	// route the timeout message back to trigger timeout certificate and new round
	mockNet.WaitReceive(t, lastTimeoutMsg)
	// This triggers TC and next round, wait for proposal
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.Equal(t, uint64(3), lastProposalMsg.Block.Round)
	require.NotNil(t, lastProposalMsg.LastRoundTc)
	require.Equal(t, uint64(2), lastProposalMsg.LastRoundTc.Timeout.Round)
	// route the proposal back
	mockNet.WaitReceive(t, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	// round 3 is skipped, as it timeouts
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.Nil(t, lastVoteMsg.LedgerCommitInfo.Hash)
}

func TestIRChangeRequestFromRootValidator_RootTimeout(t *testing.T) {
	mockNet := testnetwork.NewRootMockNetwork()
	cm, rootNode, shardNodes := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	si, err := cm.ShardInfo(partitionID, shardID)
	require.NoError(t, err)
	require.NotNil(t, si)

	// simulate IR change request message
	irChReqMsg := &abdrc.IrChangeReqMsg{
		Author: rootNode.PeerConf.ID.String(),
		IrChangeReq: &drctypes.IRChangeReq{
			Partition:  partitionID,
			CertReason: drctypes.Quorum,
			Requests:   buildBlockCertificationRequest(t, shardNodes[0:2], si.LastCR),
		},
	}
	require.NoError(t, irChReqMsg.Sign(rootNode.Signer))

	// advance round
	lastProposalMsg := testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	mockNet.WaitReceive(t, lastProposalMsg)
	mockNet.WaitReceive(t, irChReqMsg)
	voteMsg := testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	mockNet.WaitReceive(t, voteMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	// round 1 committed, round 2 quorum
	require.Equal(t, uint64(1), cm.blockStore.GetHighQc().GetCommitRound())
	require.Equal(t, uint64(2), cm.blockStore.GetHighQc().GetRound())
	require.Equal(t, uint64(3), cm.pacemaker.GetCurrentRound())
	require.Equal(t, partitionID, lastProposalMsg.Block.Payload.Requests[0].Partition)
	require.Equal(t, drctypes.Quorum, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)

	// advance round
	mockNet.WaitReceive(t, lastProposalMsg)
	lastVoteMsg := testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	mockNet.WaitReceive(t, lastVoteMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	// round 2 committed, round 3 quorum
	require.Equal(t, uint64(2), cm.blockStore.GetHighQc().GetCommitRound())
	require.Equal(t, uint64(3), cm.blockStore.GetHighQc().GetRound())
	require.Equal(t, uint64(4), cm.pacemaker.GetCurrentRound())
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// advance round with timeout
	mockNet.WaitReceive(t, lastProposalMsg)
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	// Avoid waiting for the default LocalTimeout (10s), and
	// simulate local timeout by calling the method -> race/hack accessing from different go routines not safe
	cm.onLocalTimeout(ctx)
	lastTimeoutMsg := testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.Equal(t, uint64(4), lastTimeoutMsg.Timeout.Round)
	// route the timeout message back to trigger timeout certificate and new round
	mockNet.WaitReceive(t, lastTimeoutMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	// round 2 committed, round 3 quorum - timeout cert didn't change that
	require.Equal(t, uint64(2), cm.blockStore.GetHighQc().GetCommitRound())
	require.Equal(t, uint64(3), cm.blockStore.GetHighQc().GetRound())
	require.Equal(t, uint64(4), lastProposalMsg.LastRoundTc.Timeout.Round)
	require.Equal(t, uint64(5), lastProposalMsg.Block.Round)
	require.Equal(t, uint64(5), cm.pacemaker.GetCurrentRound())
	require.NotNil(t, lastProposalMsg.LastRoundTc)

	// no changes in round 5, but...
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	// changes from round 3 are still in progress (uncommitted), since nothing gets committed with a timeout cert
	require.Equal(t, irChReqMsg.IrChangeReq.Requests[0].InputRecord, cm.blockStore.IsChangeInProgress(partitionID, shardID))

	// advance round
	mockNet.WaitReceive(t, lastProposalMsg)
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	// Check state before routing vote back to root
	getStateMsg := &abdrc.StateRequestMsg{
		NodeId: shardNodes[0].PeerConf.ID.String(),
	}
	mockNet.WaitReceive(t, getStateMsg)
	stateMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// commit head is still at round 3, as round 5 that would have committed 4 resulted in timeout
	require.Equal(t, uint64(2), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 2, len(stateMsg.Pending))
	// round 5 has been removed as it resulted in timeout quorum
	require.Equal(t, uint64(3), stateMsg.Pending[0].Round)
	require.Equal(t, uint64(5), stateMsg.Pending[1].Round)
	// send vote back to validator
	mockNet.WaitReceive(t, lastVoteMsg)
	lastProposalMsg = mockNet.WaitRootProposal(t)

	// round 2 committed, round 5 quorum
	// round 5 did not commit anything because round 4 was timeout, need two consecutive QC-s to commit
	require.Equal(t, uint64(0), cm.blockStore.GetHighQc().GetCommitRound())
	require.Equal(t, uint64(5), cm.blockStore.GetHighQc().GetRound())
	require.Equal(t, uint64(6), lastProposalMsg.Block.Round)
	require.Equal(t, uint64(6), cm.pacemaker.GetCurrentRound())
	require.Nil(t, lastProposalMsg.LastRoundTc)

	// advance round
	mockNet.WaitReceive(t, lastProposalMsg)
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(6), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotEqual(t, uint64(0), lastVoteMsg.LedgerCommitInfo.RootChainRoundNumber)
	// Check state before routing vote back to root
	mockNet.WaitReceive(t, getStateMsg)
	stateMsg = testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// commit head is still at round 2, rounds 3, 5 and 6 are added, 6 will commit 5 when it reaches quorum, but
	// this happens after vote is routed back, so current expected state is:
	require.Equal(t, uint64(2), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 3, len(stateMsg.Pending))
	// round 5 has been removed as it resulted in timeout quorum
	require.Equal(t, uint64(3), stateMsg.Pending[0].Round)
	require.Equal(t, uint64(5), stateMsg.Pending[1].Round)
	require.Equal(t, uint64(6), stateMsg.Pending[2].Round)
	mockNet.WaitReceive(t, lastVoteMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	// after two successful rounds since timeout the IR change will be finally committed and UC is returned
	result, err := readResult(cm.CertificationResult(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, partitionID, result.UnicityTreeCertificate.Partition)
	require.Nil(t, cm.blockStore.IsChangeInProgress(partitionID, shardID))

	// verify certificates have been updated when recovery query is sent
	mockNet.WaitReceive(t, getStateMsg)
	stateMsg = testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	require.Equal(t, 1, len(stateMsg.CommittedHead.ShardInfo))
	idx := slices.IndexFunc(stateMsg.CommittedHead.ShardInfo, func(c abdrc.ShardInfo) bool {
		return c.UC.UnicityTreeCertificate.Partition == partitionID
	})
	require.False(t, idx == -1)
	// at this stage the committed round is 6 and round 7 block is pending
	require.Equal(t, uint64(5), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, uint64(5), stateMsg.CommittedHead.ShardInfo[idx].UC.UnicitySeal.RootChainRoundNumber)
	require.Equal(t, 1, len(stateMsg.Pending))
	require.Equal(t, uint64(6), stateMsg.Pending[0].Round)
}

func TestIRChangeRequestFromRootValidator(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil

	mockNet := testnetwork.NewRootMockNetwork()
	cm, rootNode, shardNodes := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	si, err := cm.ShardInfo(partitionID, shardID)
	require.NoError(t, err)

	// simulate IR change request message
	irChReqMsg := &abdrc.IrChangeReqMsg{
		Author: rootNode.PeerConf.ID.String(),
		IrChangeReq: &drctypes.IRChangeReq{
			Partition:  partitionID,
			CertReason: drctypes.Quorum,
			Requests:   buildBlockCertificationRequest(t, shardNodes[0:2], si.LastCR),
		},
	}
	require.NoError(t, irChReqMsg.Sign(rootNode.Signer))

	// advance round
	mockNet.WaitReceive(t, irChReqMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	require.Equal(t, partitionID, lastProposalMsg.Block.Payload.Requests[0].Partition)
	require.Equal(t, drctypes.Quorum, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	require.Len(t, lastProposalMsg.Block.Payload.Requests[0].Requests, 2)
	// route the proposal back
	mockNet.WaitReceive(t, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(2), lastVoteMsg.VoteInfo.RoundNumber)
	require.Equal(t, uint64(1), lastVoteMsg.VoteInfo.ParentRoundNumber)
	require.Equal(t, uint64(0), lastVoteMsg.VoteInfo.Epoch)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)

	// send vote back to validator
	mockNet.WaitReceive(t, lastVoteMsg)
	// this will trigger next proposal since QC is achieved
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	// no additional requests have been received, meaning payload is empty
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())

	// route the proposal back to trigger new vote
	mockNet.WaitReceive(t, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, uint64(3), lastVoteMsg.VoteInfo.RoundNumber)
	require.NotNil(t, lastVoteMsg.LedgerCommitInfo.Hash)
	// send vote back to validator
	mockNet.WaitReceive(t, lastVoteMsg)
	// after two successful rounds the IR change will be committed and UC is returned
	result, err := readResult(cm.CertificationResult(), time.Second)
	require.NoError(t, err)

	shardConf, err := cm.orchestration.ShardConfig(partitionID, shardID, 1)
	require.NoError(t, err)
	shardConfHash, err := shardConf.Hash(crypto.SHA256)
	require.NoError(t, err)
	require.NoError(t, result.Verify(cm.trustBase, crypto.SHA256, partitionID, shardConfHash))

	// root will continue and next proposal is also triggered by the same QC
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	require.NoError(t, err)
}

func TestPartitionTimeoutFromRootValidator(t *testing.T) {
	var lastProposalMsg *abdrc.ProposalMsg = nil
	var lastVoteMsg *abdrc.VoteMsg = nil

	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, _ := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	shardConf, err := cm.orchestration.ShardConfig(partitionID, shardID, 1)
	require.NoError(t, err)

	roundNo := uint64(1) // 1 is genesis
	// run a loop of 11 rounds to produce a root chain timeout
	for i := 0; i < int(shardConf.T2Timeout/(cm.params.BlockRate/2)); i++ {
		//for i := 0; i < int(shardConf.T2Timeout/(cm.params.BlockRate/2)); i++ {
		// proposal rounds 2..
		roundNo++
		lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
		require.Empty(t, lastProposalMsg.Block.Payload.Requests)
		// route the proposal back
		mockNet.WaitReceive(t, lastProposalMsg)
		// wait for the vote message
		lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
		require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
		mockNet.WaitReceive(t, lastVoteMsg)
	}

	// proposal round 7 with timeout
	roundNo++
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.NotEmpty(t, lastProposalMsg.Block.Payload.Requests)
	require.Equal(t, drctypes.T2Timeout, lastProposalMsg.Block.Payload.Requests[0].CertReason)
	mockNet.WaitReceive(t, lastProposalMsg)
	// wait for the vote message
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	mockNet.WaitReceive(t, lastVoteMsg)
	// new proposal round 8
	roundNo++
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.Empty(t, lastProposalMsg.Block.Payload.Requests)
	mockNet.WaitReceive(t, lastProposalMsg)
	// voting round 8
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	mockNet.WaitReceive(t, lastVoteMsg)
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
	mockNet.WaitReceive(t, lastProposalMsg)
	// vote round 9
	lastVoteMsg = testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	require.Equal(t, roundNo, lastVoteMsg.VoteInfo.RoundNumber)
	mockNet.WaitReceive(t, lastVoteMsg)
	// proposal round 10
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	require.True(t, lastProposalMsg.Block.Payload.IsEmpty())
	mockNet.WaitReceive(t, lastProposalMsg)
}

func TestGetState_WithoutShards(t *testing.T) {
	mockNet := testnetwork.NewRootMockNetwork()
	rootNode := testutils.NewTestNode(t)
	trustBase := trustbase.NewTrustBaseFromVerifiers(t, map[string]abcrypto.Verifier{
		rootNode.PeerConf.ID.String(): rootNode.Verifier,
	})
	observe := testobservability.Default(t)
	orchestration := testpartition.NewOrchestration(t, observe.Logger())

	cm, err := NewConsensusManager(
		rootNode.PeerConf.ID,
		trustBase,
		orchestration,
		mockNet,
		rootNode.Signer,
		observe,
	)
	require.NoError(t, err)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// request state
	getStateMsg := &abdrc.StateRequestMsg{
		NodeId: rootNode.PeerConf.ID.String(),
	}
	mockNet.WaitReceive(t, getStateMsg)
	stateMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)

	// only genesis block present
	require.Equal(t, 0, len(stateMsg.Pending))
	require.Len(t, stateMsg.CommittedHead.ShardInfo, 0)
	// the hard-coded round 1 is the CommittedHead
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, uint64(1), stateMsg.CommittedHead.CommitQc.GetRound())
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Qc.GetRound())
	// the verification of hard-coded CommittedHead should succeed despite having no signatures
	require.Len(t, stateMsg.CommittedHead.CommitQc.Signatures, 0)
	require.NoError(t, stateMsg.Verify(crypto.SHA256, trustBase))

	// advance to round 2
	lastProposalMsg := testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)
	mockNet.WaitReceive(t, lastProposalMsg)
	voteMsg := testutils.MockAwaitMessage[*abdrc.VoteMsg](t, mockNet, network.ProtocolRootVote)
	mockNet.WaitReceive(t, voteMsg)
	lastProposalMsg = testutils.MockAwaitMessage[*abdrc.ProposalMsg](t, mockNet, network.ProtocolRootProposal)

	// request state
	mockNet.WaitReceive(t, getStateMsg)
	stateMsg = testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)

	// a new block present now, still no shards
	require.Len(t, stateMsg.Pending, 1)
	require.Equal(t, uint64(2), stateMsg.Pending[0].Round)
	// the hard-coded round 1 is still the CommittedHead
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Qc.GetRound())
	// but a new commitQc was produced with signatures
	require.Equal(t, uint64(2), stateMsg.CommittedHead.CommitQc.GetRound())
	require.Len(t, stateMsg.CommittedHead.CommitQc.Signatures, 1)
	require.NoError(t, stateMsg.Verify(crypto.SHA256, trustBase))
}

func TestGetState_WithShards(t *testing.T) {
	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, shardNodes := initConsensusManager(t, mockNet)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	getStateMsg := &abdrc.StateRequestMsg{
		NodeId: shardNodes[0].PeerConf.ID.String(),
	}
	// simulate IR change request message
	mockNet.WaitReceive(t, getStateMsg)
	// As the node is the leader, next round will trigger a proposal
	stateMsg := testutils.MockAwaitMessage[*abdrc.StateMsg](t, mockNet, network.ProtocolRootStateResp)
	// at this stage there is only genesis block
	require.Equal(t, uint64(1), stateMsg.CommittedHead.Block.Round)
	require.Equal(t, 0, len(stateMsg.Pending))
	require.Len(t, stateMsg.CommittedHead.ShardInfo, 1)
}

func Test_ConsensusManager_onVoteMsg(t *testing.T) {
	t.Parallel()

	makeVoteMsg := func(t *testing.T, cms []*ConsensusManager, round uint64) *abdrc.VoteMsg {
		t.Helper()
		qcRoundInfo := abdrctu.NewDummyRootRoundInfo(round - 2)
		commitInfo := abdrctu.NewDummyCommitInfo(t, crypto.SHA256, qcRoundInfo)
		highQc := &drctypes.QuorumCert{
			VoteInfo:         qcRoundInfo,
			LedgerCommitInfo: commitInfo,
			Signatures:       map[string]hex.Bytes{},
		}
		cib := testcertificates.UnicitySealBytes(t, commitInfo)
		for _, cm := range cms {
			sig, err := cm.safety.signer.SignBytes(cib)
			require.NoError(t, err)
			highQc.Signatures[cm.id.String()] = sig
		}

		voteRoundInfo := abdrctu.NewDummyRootRoundInfo(round)
		h, err := voteRoundInfo.Hash(crypto.SHA256)
		require.NoError(t, err)
		voteMsg := &abdrc.VoteMsg{
			VoteInfo: voteRoundInfo,
			LedgerCommitInfo: &types.UnicitySeal{
				Version:      1,
				PreviousHash: h,
			},
			HighQc: highQc,
			Author: cms[0].id.String(),
		}
		require.NoError(t, voteMsg.Sign(cms[0].safety.signer))
		return voteMsg
	}

	t.Run("stale vote", func(t *testing.T) {
		cms, _ := createConsensusManagers(t, 1, nil)
		cms[0].pacemaker.Reset(context.Background(), 8, nil, nil)
		defer cms[0].pacemaker.Stop()

		vote := makeVoteMsg(t, cms, 7)
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.EqualError(t, err, `stale vote for round 7 from `+cms[0].id.String())
		require.Empty(t, cms[0].voteBuffer)
	})

	t.Run("invalid vote: verify fails", func(t *testing.T) {
		// here we just test that only verified votes are processed, all the possible
		// vote verification failures should be tested by vote.Verify unit tests...
		const votedRound = 10
		cms, _ := createConsensusManagers(t, 1, nil)
		cms[0].pacemaker.Reset(context.Background(), votedRound-1, nil, nil)
		defer cms[0].pacemaker.Stop()

		vote := makeVoteMsg(t, cms, votedRound)
		vote.Author = "foobar"
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.EqualError(t, err, `invalid vote: vote from 'foobar' signature verification error: author 'foobar' is not part of the trust base`)
		require.Empty(t, cms[0].voteBuffer)
	})

	t.Run("vote for next round should be buffered", func(t *testing.T) {
		const votedRound = 10
		// need at least two CMs so that we do not trigger recovery because of having
		// received enough votes for the quorum
		cms, _ := createConsensusManagers(t, 2, nil)
		cms[0].pacemaker.Reset(context.Background(), votedRound-1, nil, nil)
		defer cms[0].pacemaker.Stop()

		vote := makeVoteMsg(t, cms, votedRound+1)
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.NoError(t, err)
		require.Equal(t, vote, cms[0].voteBuffer[vote.Author])
	})

	t.Run("repeat vote for next round should be ignored (not buffered twice)", func(t *testing.T) {
		const votedRound = 10
		// need at least two CMs so that we do not trigger recovery because of having
		// received enough votes for the quorum
		cms, _ := createConsensusManagers(t, 2, nil)
		cms[0].pacemaker.Reset(context.Background(), votedRound-1, nil, nil)
		defer cms[0].pacemaker.Stop()

		vote := makeVoteMsg(t, cms, votedRound+1)
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.NoError(t, err)
		require.Equal(t, vote, cms[0].voteBuffer[vote.Author])
		// send the vote again - should not trigger recovery ie vote is not counted again
		require.NoError(t, cms[0].onVoteMsg(context.Background(), vote))
		require.Equal(t, vote, cms[0].voteBuffer[vote.Author], "expected original vote still to be in the buffer")
		require.Len(t, cms[0].voteBuffer, 1, "expected only one vote to be buffered")
	})

	t.Run("quorum of votes for next round should trigger recovery", func(t *testing.T) {
		const votedRound = 10
		cms, _ := createConsensusManagers(t, 1, nil)
		cms[0].pacemaker.Reset(context.Background(), votedRound-1, nil, nil)
		defer cms[0].pacemaker.Stop()

		// as we have single CM vote means quorum and recovery should be triggered as CM hasn't
		// seen proposal yet
		vote := makeVoteMsg(t, cms, votedRound+1)
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.EqualError(t, err, `have received 1 votes but no proposal, entering recovery`)
		require.Equal(t, vote, cms[0].voteBuffer[vote.Author], "expected vote to be buffered")
	})
	/* todo - need a way to mock storage
	t.Run("not the leader of the (next) round", func(t *testing.T) {
		const votedRound = 10
		cms, _, _ := createConsensusManagers(t, 2, []*genesis.PartitionRecord{partitionRecord})
		cms[0].leaderSelector = constLeader{leader: cms[1].id, nodes: cms[1].leaderSelector.GetNodes()} // make sure this CM won't be the leader
		cms[0].pacemaker.Reset(context.Background(), votedRound-1, nil, nil)
		defer cms[0].pacemaker.Stop()

		vote := makeVoteMsg(t, cms, votedRound)
		err := cms[0].onVoteMsg(context.Background(), vote)
		require.EqualError(t, err, fmt.Sprintf("validator is not the leader for round %d", votedRound+1))
		require.Empty(t, cms[0].voteBuffer)
	})
	*/
}

func Test_ConsensusManager_handleRootNetMsg(t *testing.T) {
	t.Parallel()

	observe := testobservability.Default(t)
	pm, err := NewPacemaker(time.Minute, 2*time.Minute, observe)
	if err != nil {
		t.Fatalf("creating Pacemaker: %v", err)
	}

	t.Run("untyped nil msg", func(t *testing.T) {
		cm := &ConsensusManager{pacemaker: pm, tracer: observe.Tracer("cm")}
		require.NoError(t, cm.initMetrics(observe))
		err := cm.handleRootNetMsg(context.Background(), nil)
		require.EqualError(t, err, `unknown message type <nil>`)
	})

	t.Run("type not known for the handler", func(t *testing.T) {
		cm := &ConsensusManager{pacemaker: pm, tracer: observe.Tracer("cm")}
		require.NoError(t, cm.initMetrics(observe))
		err := cm.handleRootNetMsg(context.Background(), "foobar")
		require.EqualError(t, err, `unknown message type string`)
	})
}

func Test_ConsensusManager_messages(t *testing.T) {
	t.Parallel()

	waitExit := func(t *testing.T, ctxCancel context.CancelFunc, doneCh chan struct{}) {
		t.Helper()
		ctxCancel()
		// and wait for cm to exit
		select {
		case <-time.After(10 * time.Second):
			_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			t.Fatal("consensus manager did not exit in time")
		case <-doneCh:
		}
	}

	// partition data used/shared by tests
	shardNodes, shardNodeInfos := testutils.CreateTestNodes(t, 2)

	t.Run("IR change request from shard included in proposal", func(t *testing.T) {
		cms, rootNet := createConsensusManagers(t, 1, shardNodeInfos)

		ctx, stopCM := context.WithCancel(context.Background())
		defer stopCM()
		go func() { require.ErrorIs(t, cms[0].Run(ctx), context.Canceled) }()

		// proposal will be broadcast so eavesdrop the network and make copy of it
		propCh := make(chan *abdrc.ProposalMsg, 1)
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			if msg, ok := msg.(*abdrc.ProposalMsg); ok {
				propCh <- msg
			}
			return false
		})

		si, err := cms[0].ShardInfo(partitionID, shardID)
		require.NoError(t, err)

		// simulate root validator node sending IRCR to consensus manager
		irCReq := IRChangeRequest{
			Partition: partitionID,
			Reason:    Quorum,
			Requests:  buildBlockCertificationRequest(t, shardNodes, si.LastCR),
		}

		rcCtx, rcCancel := context.WithTimeout(ctx, cms[0].pacemaker.minRoundLen)
		require.NoError(t, cms[0].RequestCertification(rcCtx, irCReq), "CM doesn't consume IR change request")
		rcCancel()

		// IRCR must be included into proposal
		select {
		case <-time.After(cms[0].pacemaker.maxRoundLen):
			t.Fatal("haven't got the proposal before timeout")
		case prop := <-propCh:
			require.NotNil(t, prop)
			require.NotNil(t, prop.Block)
			require.NotNil(t, prop.Block.Payload)
			require.Len(t, prop.Block.Payload.Requests, 1)
			require.EqualValues(t, irCReq.Partition, prop.Block.Payload.Requests[0].Partition)
			require.ElementsMatch(t, irCReq.Requests, prop.Block.Payload.Requests[0].Requests)
		}
	})

	t.Run("IR change request from partition forwarded to leader", func(t *testing.T) {
		// we create two CMs but only non-leader node has to be running as we test
		// that it will forward message to leader by monitoring network
		cms, rootNet := createConsensusManagers(t, 2, shardNodeInfos)
		cmLeader := cms[0]
		nonLeaderNode := cms[1]
		nonLeaderNode.leaderSelector = constLeader{leader: cmLeader.id, nodes: cmLeader.leaderSelector.GetNodes()} // use "const leader" to take leader selection out of test
		// eavesdrop the network and copy IR change message sent by non-leader to leader
		irCh := make(chan *abdrc.IrChangeReqMsg, 1)
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			if msg, ok := msg.(*abdrc.IrChangeReqMsg); ok && from == nonLeaderNode.id && to == cmLeader.id {
				irCh <- msg
			}
			return false
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); require.ErrorIs(t, nonLeaderNode.Run(ctx), context.Canceled) }()
		defer waitExit(t, cancel, done)

		// simulate partition request sent to non-leader node
		irCReq := IRChangeRequest{
			Partition: partitionID,
			Reason:    Quorum,
			Requests:  buildBlockCertificationRequest(t, shardNodes, nil),
		}
		rcCtx, rcCancel := context.WithTimeout(ctx, nonLeaderNode.pacemaker.minRoundLen)
		require.NoError(t, nonLeaderNode.RequestCertification(rcCtx, irCReq), "CM doesn't consume IR change request")
		rcCancel()

		select {
		case <-time.After(cmLeader.pacemaker.maxRoundLen):
			t.Fatal("haven't got the IR Change message before timeout")
		case irMsg := <-irCh:
			require.NotNil(t, irMsg)
			require.Equal(t, irMsg.Author, nonLeaderNode.id.String())
			require.EqualValues(t, irCReq.Partition, irMsg.IrChangeReq.Partition)
			require.ElementsMatch(t, irCReq.Requests, irMsg.IrChangeReq.Requests)
		}
	})

	t.Run("IR change request forwarded by peer included in proposal", func(t *testing.T) {
		// we create two CMs but only leader is running, the other is just needed for
		// valid peer ID in the genesis so IRCR can be signed and validated
		cms, rootNet := createConsensusManagers(t, 2, shardNodeInfos)

		cmOther := cms[1]
		cmLeader := cms[0]
		cmLeader.leaderSelector = constLeader{leader: cmLeader.id, nodes: cmLeader.leaderSelector.GetNodes()} // use "const leader" to take leader selection out of test

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cmLeader.Run(ctx), context.Canceled) }()

		// leader is expected to broadcast proposal message, snoop for it
		wire := make(chan *abdrc.ProposalMsg, 1)
		rootNet.SetFirewall(ForwardMsgs(cmLeader.id, cmOther.id, wire))

		si, err := cmLeader.ShardInfo(partitionID, shardID)
		require.NoError(t, err)

		// simulate IR change request message, "other root node" forwarding IRCR to leader
		irChReqMsg := &abdrc.IrChangeReqMsg{
			Author: cmOther.id.String(),
			IrChangeReq: &drctypes.IRChangeReq{
				Partition:  partitionID,
				CertReason: drctypes.Quorum,
				Requests:   buildBlockCertificationRequest(t, shardNodes[0:2], si.LastCR),
			},
		}
		require.NoError(t, cmOther.safety.Sign(irChReqMsg))
		require.NoError(t, cmOther.net.Send(ctx, irChReqMsg, cmLeader.id))

		// IRCR must be included into broadcast proposal, either this or next round
		sawIRCR := false
		for cnt := 0; cnt < 2 && !sawIRCR; cnt++ {
			select {
			case <-time.After(cmLeader.pacemaker.maxRoundLen):
				t.Fatal("haven't got the proposal before timeout")
			case prop := <-wire:
				require.NotNil(t, prop)
				require.NotNil(t, prop.Block)
				require.NotNil(t, prop.Block.Payload)
				if len(prop.Block.Payload.Requests) == 1 {
					require.EqualValues(t, irChReqMsg.IrChangeReq.Partition, prop.Block.Payload.Requests[0].Partition)
					require.ElementsMatch(t, irChReqMsg.IrChangeReq.Requests, prop.Block.Payload.Requests[0].Requests)
					sawIRCR = true
				}
			}
		}
		require.True(t, sawIRCR, "expected to see the IRCR in one of the next two proposals")
	})

	t.Run("IR change request arrives late and is forwarded to the next leader", func(t *testing.T) {
		// mimic situation where nonLeaderNode was the leader and IRCR was sent to it. However, by
		// the time msg arrives leader has changed to cmLeader, so we expect nonLeaderNode to
		// forward the message.
		cms, rootNet := createConsensusManagers(t, 2, shardNodeInfos)
		cmLeader := cms[0]
		nonLeaderNode := cms[1]
		nonLeaderNode.leaderSelector = constLeader{leader: cmLeader.id, nodes: cmLeader.leaderSelector.GetNodes()}

		irCh := make(chan *abdrc.IrChangeReqMsg, 1)
		rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
			if msg, ok := msg.(*abdrc.IrChangeReqMsg); ok && from == nonLeaderNode.id && to == cmLeader.id {
				irCh <- msg
			}
			return false
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); require.ErrorIs(t, nonLeaderNode.Run(ctx), context.Canceled) }()
		defer waitExit(t, cancel, done)

		// send IRCR to non-leader, simulating message arriving late, leader has changed
		irChReqMsg := &abdrc.IrChangeReqMsg{
			Author: cmLeader.id.String(),
			IrChangeReq: &drctypes.IRChangeReq{
				Partition:  partitionID,
				CertReason: drctypes.Quorum,
				Requests:   buildBlockCertificationRequest(t, shardNodes[0:2], nil),
			},
		}
		require.NoError(t, cmLeader.safety.Sign(irChReqMsg))
		rootNet.Send(irChReqMsg, nonLeaderNode.id)

		// non-leader is not the next leader and must forward the request to the leader node
		select {
		case <-time.After(cmLeader.pacemaker.maxRoundLen):
			t.Fatal("haven't seen forwarded proposal before timeout")
		case irMsg := <-irCh:
			require.NotNil(t, irMsg)
			require.Equal(t, irMsg.Author, cmLeader.id.String())
			require.EqualValues(t, irChReqMsg.IrChangeReq.Partition, irMsg.IrChangeReq.Partition)
			require.ElementsMatch(t, irChReqMsg.IrChangeReq.Requests, irMsg.IrChangeReq.Requests)
		}
	})

	t.Run("state request triggers response", func(t *testing.T) {
		cms, _ := createConsensusManagers(t, 2, shardNodeInfos)
		cmA, cmB := cms[0], cms[1]

		// only launch cmA, we manage cmB "manually"
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); require.ErrorIs(t, cmA.Run(ctx), context.Canceled) }()
		defer waitExit(t, cancel, done)

		// cmB sends state request to cmA
		msg := &abdrc.StateRequestMsg{NodeId: cmB.id.String()}
		require.NoError(t, cmB.net.Send(ctx, msg, cmA.id))

		// cmB should receive state response
		select {
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("timeout while waiting for recovery response")
		case msg := <-cmB.net.ReceivedChannel():
			state := msg.(*abdrc.StateMsg)
			require.NotNil(t, state)
			require.EqualValues(t, 1, state.CommittedHead.Block.Round)
			require.Empty(t, state.Pending)
			require.Len(t, state.CommittedHead.ShardInfo, 1)
		}
	})
}

func Test_ConsensusManager_sendCertificates(t *testing.T) {
	t.Parallel()

	// generate UCs for given systems (with random data in QC)
	makeUCs := func(partitionIds ...types.PartitionID) map[types.PartitionShardID]*certification.CertificationResponse {
		rUC := make(map[types.PartitionShardID]*certification.CertificationResponse)
		for _, id := range partitionIds {
			cr := certification.CertificationResponse{
				Partition: id,
				Shard:     shardID,
				UC: types.UnicityCertificate{
					Version:       1,
					ShardConfHash: test.RandomBytes(32),
					UnicityTreeCertificate: &types.UnicityTreeCertificate{
						Version:   1,
						Partition: id,
					},
				},
			}
			rUC[types.PartitionShardID{PartitionID: cr.Partition, ShardID: cr.Shard.Key()}] = &cr
		}
		return rUC
	}

	// consumeUCs reads UCs from "cm"-s output and stores them into map it returns.
	// it reads until "timeout" has passed.
	consumeUCs := func(cm *ConsensusManager, timeout time.Duration) map[types.PartitionShardID]*certification.CertificationResponse {
		to := time.After(timeout)
		rUC := make(map[types.PartitionShardID]*certification.CertificationResponse)
		for {
			select {
			case uc := <-cm.CertificationResult():
				rUC[types.PartitionShardID{PartitionID: uc.Partition, ShardID: uc.Shard.Key()}] = uc
			case <-to:
				return rUC
			}
		}
	}

	outputMustBeEmpty := func(t *testing.T, cm *ConsensusManager) {
		t.Helper()
		select {
		case uc := <-cm.CertificationResult():
			t.Errorf("unexpected data from cert chan: %#v", uc)
		default:
		}
	}

	t.Run("consume before next input is sent", func(t *testing.T) {
		cms, _ := createConsensusManagers(t, 1, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cms[0].sendCertificates(ctx), context.Canceled) }()

		// send some certificates into sink...
		ucs := makeUCs(1, 2)
		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		default:
			t.Fatal("expected that input would be accepted immediately, sink should be empty")
		}
		//...and consume them
		rUC := consumeUCs(cms[0], 100*time.Millisecond)
		require.Equal(t, ucs, rUC)
		outputMustBeEmpty(t, cms[0])

		// and repeat the exercise with different partition identifiers
		ucs = makeUCs(3, 4)
		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		default:
			t.Fatal("expected that input would be accepted immediately, sink should be empty")
		}

		rUC = consumeUCs(cms[0], 100*time.Millisecond)
		require.Equal(t, ucs, rUC)
		outputMustBeEmpty(t, cms[0])
	})

	t.Run("overwriting unconsumed QC", func(t *testing.T) {
		cms, _ := createConsensusManagers(t, 1, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cms[0].sendCertificates(ctx), context.Canceled) }()

		// exp - expected result in the end of test, we add/overwrite certs as we send them
		exp := map[types.PartitionShardID]*certification.CertificationResponse{}

		ucs := makeUCs(1, 2)
		for k, v := range ucs {
			exp[k] = v
		}

		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		default:
			t.Fatal("expected that input would be accepted immediately, sink should be empty")
		}

		// as we haven't consumed anything sending new set of certs into the sink should
		// overwrite {0,0,0,2} and add {0,0,0,3}
		ucs = makeUCs(3, 2)
		for k, v := range ucs {
			exp[k] = v
		}

		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		case <-time.After(100 * time.Millisecond):
			t.Error("next input hasn't been consumed fast enough")
		}

		rUC := consumeUCs(cms[0], 100*time.Millisecond)
		require.Len(t, rUC, 3, "number of different partition identifiers")
		require.Equal(t, exp, rUC)
		outputMustBeEmpty(t, cms[0])
	})

	t.Run("adding without overwriting", func(t *testing.T) {
		cms, _ := createConsensusManagers(t, 1, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { require.ErrorIs(t, cms[0].sendCertificates(ctx), context.Canceled) }()
		// exp - expected result in the end of test, we add/overwrite certs as we send them
		exp := map[types.PartitionShardID]*certification.CertificationResponse{}

		ucs := makeUCs(1, 2)
		for k, v := range ucs {
			exp[k] = v
		}

		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		default:
			t.Fatal("expected that input would be accepted immediately, sink should be empty")
		}

		// send another set of certs, unique sysIDs, ie no overwrites, just add (nothing
		// has been consumed yet)
		ucs = makeUCs(3, types.PartitionID(4))
		for k, v := range ucs {
			exp[k] = v
		}

		select {
		case cms[0].ucSink <- slices.Collect(maps.Values(ucs)):
		case <-time.After(100 * time.Millisecond):
			t.Error("next input hasn't been consumed fast enough")
		}

		rUC := consumeUCs(cms[0], 100*time.Millisecond)
		require.Len(t, rUC, 4, "number of different partition identifiers")
		require.Equal(t, exp, rUC)
		outputMustBeEmpty(t, cms[0])
	})

	t.Run("concurrency", func(t *testing.T) {
		// concurrent read and writes to trip race detector
		cms, _ := createConsensusManagers(t, 1, nil)
		done := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer close(done)
			require.ErrorIs(t, cms[0].sendCertificates(ctx), context.Canceled)
		}()

		go func() {
			for {
				select {
				case cms[0].ucSink <- slices.Collect(maps.Values(makeUCs(1, 2, 3))):
				case <-ctx.Done():
					return
				}
			}
		}()

		go func() {
			for {
				select {
				case <-cms[0].CertificationResult():
				case <-ctx.Done():
					return
				}
			}
		}()

		time.Sleep(time.Second)
		cancel()
		<-done
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

		nodes = selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{}, 2)
		require.Empty(t, nodes)
	})

	t.Run("no duplicates added", func(t *testing.T) {
		nodes := selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{idA.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idA}, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{idA.String(): nil, idB.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idA, idB}, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{idA.String(): nil, idB.String(): nil}, 3)
		require.ElementsMatch(t, []peer.ID{idA, idB}, nodes)
	})

	t.Run("invalid IDs are ignored", func(t *testing.T) {
		nodes := selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{"foo bar": nil}, 1)
		require.Empty(t, nodes)

		nodes = selectRandomNodeIdsFromSignatureMap(map[string]hex.Bytes{"foo bar": nil, idB.String(): nil}, 2)
		require.ElementsMatch(t, []peer.ID{idB}, nodes)
	})

	t.Run("max count items is returned", func(t *testing.T) {
		inp := map[string]hex.Bytes{idA.String(): nil, idB.String(): nil, idC.String(): nil}

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

	// for quorum, we need 2/3+1 validators (root nodes) to be healthy
	const rootNodeCnt = 4
	// destination round - until which round (minimum) the test should run. test stops as soon as
	// one node is in that round so last round might be "incomplete". System starts with round 2.
	const destRound = 10

	// consumeUC acts as a validator node consuming the UC-s generated by CM (until ctx is cancelled).
	// strictly speaking not needed as current implementation should continue working even when there
	// is no-one consuming UCs.
	consumeUC := func(ctx context.Context, cm *ConsensusManager) {
		for {
			select {
			case <-ctx.Done():
				return
			case uc := <-cm.certResultCh:
				t.Logf("%s UC for round %d sent to validator", cm.id.ShortString(), uc.UC.GetRoundNumber())
			}
		}
	}

	cms, rootNet := createConsensusManagers(t, rootNodeCnt, nil)

	var totalMsgCnt atomic.Uint32
	rootNet.SetFirewall(func(from, to peer.ID, msg any) bool {
		msgCnt := totalMsgCnt.Add(1)
		block := msgCnt%200 == 0 // drop every n-th message from the network
		t.Logf("%t # %s -> %s : %T", block, from.ShortString(), to.ShortString(), msg)
		return block
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	start := time.Now()
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(len(cms))
		for _, v := range cms {
			go func(cm *ConsensusManager) {
				defer wg.Done()
				require.ErrorIs(t, cm.Run(ctx), context.Canceled)
			}(v)
			go func(cm *ConsensusManager) { consumeUC(ctx, cm) }(v)
		}
		wg.Wait()
		close(done)
	}()
	cm := cms[0]
	// assume rounds are successful and each round takes between minRoundLen and roundTimeout on average
	maxTestDuration := destRound * (cm.pacemaker.minRoundLen + (cm.pacemaker.maxRoundLen-cm.pacemaker.minRoundLen)/2)
	require.Eventually(t, func() bool { return cm.pacemaker.GetCurrentRound() >= destRound }, maxTestDuration, 100*time.Millisecond, "waiting for round %d to be achieved", destRound)
	stop := time.Now()
	cancel()
	// when calculating expected message counts keep in mind that last round might not be complete
	// ie the test ended before all nodes had a chance to post their message. so use -1 rounds!
	// and we start from round 2 so that's another -1 completed rounds.
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
	// wait for cm routine to exit, otherwise logger may be destructed before last usage
	select {
	case <-time.After(1000 * time.Millisecond):
		t.Fatal("consensus managers did not exit in time")
	case <-done:
	}
}

func Test_ConsensusManager_RestoreVote(t *testing.T) {
	// Make timeout 10x shorter
	consensusParams := NewConsensusParams()
	consensusParams.BlockRate /= 10
	consensusParams.LocalTimeout /= 10

	mockNet := testnetwork.NewRootMockNetwork()
	cm, _, _ := initConsensusManager(t, mockNet, WithConsensusParams(*consensusParams))

	timeoutVote := &abdrc.TimeoutMsg{
		Timeout: &drctypes.Timeout{Round: 2},
		Author:  "test",
		LastTC: &drctypes.TimeoutCert{
			Timeout: &drctypes.Timeout{Round: 1},
		},
	}
	// store timeout vote to DB
	require.NoError(t, storage.WriteVote(cm.blockStore.GetDB(), timeoutVote))

	// replace leader selector
	allNodes := cm.leaderSelector.GetNodes()
	leaderId, err := p2ptest.RandPeerID()
	require.NoError(t, err)
	allNodes = append(allNodes, leaderId)
	cm.leaderSelector = constLeader{leader: leaderId, nodes: allNodes}
	require.NoError(t, err)
	require.NotNil(t, cm)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()
	lastTimeoutMsg := testutils.MockAwaitMessage[*abdrc.TimeoutMsg](t, mockNet, network.ProtocolRootTimeout)
	require.NotNil(t, lastTimeoutMsg)
	// make sure the stored timeout vote is broadcast
	require.EqualValues(t, 2, lastTimeoutMsg.Timeout.Round)
	require.EqualValues(t, "test", lastTimeoutMsg.Author)
	require.EqualValues(t, 1, lastTimeoutMsg.LastTC.GetRound())
}
