package rootvalidator

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rootchain"

	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/leader"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	RootNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}
	// Leader provides interface to different leader selection algorithms
	Leader interface {
		// IsValidLeader returns valid leader for round/view number
		IsValidLeader(author peer.ID, round uint64) bool
		// GetLeaderForRound returns valid leader (node id) for round/view number
		GetLeaderForRound(round uint64) peer.ID
		// GetRootNodes returns all nodes
		GetRootNodes() []peer.ID
	}

	AtomicBroadcastManager struct {
		peer         *network.Peer
		store        StateStore
		timers       *timer.Timers
		net          RootNet
		roundState   *RoundState
		proposer     Leader
		rootVerifier *RootNodeVerifier
		safety       *SafetyModule
		stateLedger  *StateLedger
	}
)

func NewAtomicBroadcastManager(host *network.Peer, conf *RootNodeConf, stateStore StateStore,
	partitionStore *rootchain.PartitionStore, safetyModule *SafetyModule, net RootNet) (*AtomicBroadcastManager, error) {
	// Sanity checks
	if conf == nil {
		return nil, errors.New("cannot start atomic broadcast handler, conf is nil")
	}
	if net == nil {
		return nil, errors.New("network is nil")
	}
	selfId := host.ID().String()
	log.SetContext(log.KeyNodeID, selfId)
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", host.ID(), host.MultiAddresses())
	// Get node id's from cluster configuration and sort by alphabet
	// All root validator nodes, node id to public key map
	rootNodeIds := make([]peer.ID, 0, len(conf.RootTrustBase))
	for id := range conf.RootTrustBase {
		rootNodeIds = append(rootNodeIds, peer.ID(id))
	}
	lastState, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	l, err := leader.NewRotatingLeader(rootNodeIds, 1)
	if err != nil {
		return nil, err
	}
	rootVerifier, err := NewRootClusterVerifier(conf.RootTrustBase, conf.ConsensusThreshold)
	if err != nil {
		return nil, err
	}
	roundState := NewRoundState(lastState.LatestRound, conf.LocalTimeoutMs)
	timers := timer.NewTimers()
	timers.Start(localTimeoutId, roundState.GetRoundTimeout())
	// read state
	state, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	ledger, err := NewStateLedger(stateStore, partitionStore, conf.HashAlgorithm)
	if err != nil {
		return nil, err
	}
	// Am I the next leader?
	if l.GetLeaderForRound(state.LatestRound+1) == host.ID() {
		// Start timer and wait for requests from partition nodes
		timers.Start(blockRateId, conf.BlockRateMs)
	}
	return &AtomicBroadcastManager{
		peer:         host,
		store:        stateStore,
		timers:       timers,
		net:          net,
		roundState:   roundState,
		proposer:     l,
		rootVerifier: rootVerifier,
		safety:       safetyModule,
		stateLedger:  ledger,
	}, nil
}

func (a *AtomicBroadcastManager) Timers() *timer.Timers {
	return a.timers
}

func (a *AtomicBroadcastManager) Receive() <-chan network.ReceivedMessage {
	return a.net.ReceivedChannel()
}

func (a *AtomicBroadcastManager) OnAtomicBroadcastMessage(msg *network.ReceivedMessage) {
	if msg.Message == nil {
		logger.Warning("Root network received message is nil")
		return
	}
	switch msg.Protocol {
	case network.ProtocolRootIrChangeReq:
		req, correctType := msg.Message.(*atomic_broadcast.IRChangeReqMsg)
		if !correctType {
			logger.Warning("Type %T not supported", msg.Message)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling IR Change Request from root validator node"), req)
		logger.Debug("IR change request from %v", msg.From)
		// Am I the next leader or current leader and have not yet proposed? If not, ignore.
		// Buffer and wait for opportunity to make the next proposal
		// Todo: Add handling

		break
	case network.ProtocolRootProposal:
		req, correctType := msg.Message.(*atomic_broadcast.ProposalMsg)
		if !correctType {
			logger.Warning("Type %T not supported", msg.Message)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling Proposal from %s", req.Block.Author), req)
		logger.Debug("Proposal received from %v", msg.From)
		a.onProposalMsg(req)
		break
	case network.ProtocolRootVote:
		req, correctType := msg.Message.(*atomic_broadcast.VoteMsg)
		if !correctType {
			logger.Warning("Type %T not supported", msg.Message)
		}
		logger.Debug("Vote received from %v", msg.From)
		a.onVoteMsg(req)
		break
	case network.ProtocolRootStateReq:
		req, correctType := msg.Message.(*atomic_broadcast.StateRequestMsg)
		if !correctType {
			logger.Warning("Type %T not supported", msg.Message)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Received block request"), req)
		logger.Debug("State request from", msg.From)
		// Send state, with proof (signed by other validators)
		// Todo: add handling
		break
	case network.ProtocolRootStateResp:
		req, correctType := msg.Message.(*atomic_broadcast.StateReplyMsg)
		if !correctType {
			logger.Warning("Type %T not supported", msg.Message)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Received block"), req)
		logger.Debug("State reply from %v", msg.From)
		// Verify and store
		// Todo: Add handling
		break
	default:
		logger.Warning("Unknown protocol req %s from %v", msg.Protocol, msg.From)
		break
	}
}

// OnTimeout handle timeouts
func (a *AtomicBroadcastManager) OnTimeout(timerId string) {
	// always restart timer
	defer a.timers.Restart(timerId)
	logger.Debug("Handling timeout %s", timerId)
	// Has the validator voted in this round
	voteMsg := a.roundState.GetVoted()
	if voteMsg == nil {
		// todo: generate empty proposal and execute it (not yet specified)
		// remove below lines once able to generate a empty proposal and execute
		return
	}
	// here voteMsg is not nil
	// either validator has already voted in this round already or a dummy vote was created
	if !voteMsg.IsTimeout() {
		// add timeout with signature and resend vote
		timeout := voteMsg.NewTimeout(a.stateLedger.HighQC)
		sig, err := a.safety.SignTimeout(timeout, a.roundState.LastRoundTC())
		if err != nil {
			logger.Warning("Local timeout error")
			return
		}
		// Add timeout to vote
		if err := voteMsg.AddTimeoutSignature(timeout, sig); err != nil {
			logger.Warning("Atomic broadcast local timeout handing failed: %v", err)
			return
		}
	}
	// Record vote
	a.roundState.SetVoted(voteMsg)
	// broadcast timeout vote
	allValidators := a.proposer.GetRootNodes()
	receivers := make([]peer.ID, len(allValidators))
	receivers = append(receivers, allValidators...)
	err := a.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send vote message: %v", err)
	}
}

// OnIRChange handle timeouts
func (a *AtomicBroadcastManager) OnIRChange(irChange *atomic_broadcast.IRChangeReqMsg) {
	// Am I the next leader
	// todo: I am leader now, but have not yet proposed, should still accept requests - throttling
	if a.proposer.IsValidLeader(a.peer.ID(), a.roundState.GetCurrentRound()+1) == false {
		logger.Warning("Validator is not leader in next round %v, IR change req ignored",
			a.roundState.GetCurrentRound()+1)
		return
	}
	// Store

}

// onVoteMsg handle votes and timeout votes
func (a *AtomicBroadcastManager) onVoteMsg(vote *atomic_broadcast.VoteMsg) {
	util.WriteDebugJsonLog(logger, fmt.Sprintf("Received Vote from %s", vote.Author), vote)
	// verify signature on vote
	err := vote.Verify(a.rootVerifier)
	if err != nil {
		logger.Warning("Atomic broadcast Vote verify failed: %v", err)
	}
	// Todo: Check state and synchronize
	// Was the proposal received? If not, should we recover it? Or should it be included in the vote?

	round := vote.VoteInfo.RootRound
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	if vote.IsTimeout() == false {
		nextRound := round + 1
		// verify that the validator is correct leader in next round
		if a.proposer.IsValidLeader(a.peer.ID(), nextRound) == false {
			logger.Warning("Received vote, validator is not leader in next round %v, vote ignored", nextRound)
			return
		}
	}
	// Store vote, check for QC and TC.
	qc, tc := a.roundState.RegisterVote(vote, a.rootVerifier)
	if qc != nil {
		logger.Warning("Round %v quorum achieved", vote.VoteInfo.RootRound)
		// advance round
		a.processCertificateQC(qc)
	}
	if tc != nil {
		logger.Warning("Round %v timeout quorum achieved", vote.VoteInfo.RootRound)
		a.roundState.AdvanceRoundTC(tc)
	}
	// This node is the new leader in this round/view, generate proposal
	a.processNewRoundEvent()
}

func (a *AtomicBroadcastManager) onProposalMsg(proposal *atomic_broadcast.ProposalMsg) {
	util.WriteDebugJsonLog(logger, fmt.Sprintf("Received Proposal from %s", proposal.Block.Author), proposal)
	// verify signature on proposal
	err := proposal.Verify(a.rootVerifier)
	if err != nil {
		logger.Warning("Atomic broadcast proposal verify failed: %v", err)
	}
	logger.Info("Received proposal message from %v", proposal.Block.Author)
	// Every proposal must carry a QC for previous round
	// Process QC first
	a.processCertificateQC(proposal.Block.Qc)
	a.processCertificateQC(proposal.HighCommitQc)
	a.roundState.AdvanceRoundTC(proposal.LastRoundTc)
	// Is from valid proposer
	if a.proposer.IsValidLeader(peer.ID(proposal.Block.Author), proposal.Block.Round) == false {
		logger.Warning("Proposal author %V is not a valid proposer for round %v, ignoring proposal",
			proposal.Block.Author, proposal.Block.Round)
		return
	}
	// Proposal is
	// Check state
	if !bytes.Equal(proposal.Block.Qc.VoteInfo.ExecStateId, a.stateLedger.GetCurrentStateHash()) {
		logger.Error("Recovery not yet implemented")
	}
	// speculatively execute proposal
	err = a.stateLedger.ExecuteProposalPayload(a.roundState.GetCurrentRound(), proposal.Block.Payload)
	if err != nil {
		logger.Warning("Failed to execute proposal: %v", err.Error())
		// cannot send vote, so just return anc wait for local timeout or new proposal (and recover then)
		return
	}
	executionResult := a.stateLedger.ProposedState()
	if err := executionResult.IsValid(); err != nil {
		logger.Warning("Failed to execute proposal: %v", err.Error())
		return
	}
	// send vote
	vote := &atomic_broadcast.VoteMsg{
		VoteInfo: &atomic_broadcast.VoteInfo{
			BlockId:       proposal.Block.Id,
			RootRound:     proposal.Block.Round,
			Epoch:         proposal.Block.Epoch,
			ParentBlockId: proposal.Block.Qc.VoteInfo.BlockId,
			ParentRound:   proposal.Block.Qc.VoteInfo.RootRound,
			ExecStateId:   a.stateLedger.GetProposedStateHash(),
		},
		HighCommitQc:     a.stateLedger.HighCommitQC,
		Author:           string(a.peer.ID()),
		TimeoutSignature: nil,
	}
	if err := a.safety.SignVote(vote, a.roundState.LastRoundTC()); err != nil {
		logger.Warning("Failed to sign vote, vote not sent: %v", err.Error())
		return
	}
	a.roundState.SetVoted(vote)
	// send vote to the next leader
	receivers := make([]peer.ID, 1)
	receivers = append(receivers, a.proposer.GetLeaderForRound(a.roundState.GetCurrentRound()+1))
	err = a.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  vote,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send vote message: %v", err)
	}
}
func (a *AtomicBroadcastManager) processCertificateQC(qc *atomic_broadcast.QuorumCert) {
	a.stateLedger.ProcessQc(qc)
	a.roundState.AdvanceRoundQC(qc)
}

func (a *AtomicBroadcastManager) processNewRoundEvent() {

}
