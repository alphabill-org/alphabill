package distributed

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed/leader"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// local timeout
	blockRateID    = "block-rate"
	localTimeoutID = "local-timeout"
)

type (
	RootNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}
	// Leader provides interface to different leader selection algorithms
	Leader interface {
		// GetLeaderForRound returns valid leader (node id) for round/view number
		GetLeaderForRound(round uint64) peer.ID
		// GetRootNodes returns all nodes
		GetRootNodes() []*network.PeerInfo
	}

	PartitionStore interface {
		Info(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	StateStore interface {
		Save(state *store.RootState) error
		Get() (*store.RootState, error)
	}

	AbConsensusConfig struct {
		BlockRateMs        time.Duration
		LocalTimeoutMs     time.Duration
		ConsensusThreshold uint32
		RootTrustBase      map[string]crypto.Verifier
		HashAlgorithm      gocrypto.Hash
	}

	ConsensusManager struct {
		ctx            context.Context
		ctxCancel      context.CancelFunc
		certReqCh      chan consensus.IRChangeRequest
		certResultCh   chan certificates.UnicityCertificate
		config         *AbConsensusConfig
		peer           *network.Peer
		timers         *timer.Timers
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		rootVerifier   *RootNodeVerifier
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		roundPipeline  *RoundPipeline
		partitions     PartitionStore
		stateStore     StateStore
		waitPropose    bool
	}
)

func loadConf(genesisRoot *genesis.GenesisRootRecord) *AbConsensusConfig {
	rootTrustBase, err := genesis.NewValidatorTrustBase(genesisRoot.RootValidators)
	if err != nil {
		return nil
	}
	conf := &AbConsensusConfig{
		BlockRateMs:        time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeoutMs:     time.Duration(genesisRoot.Consensus.ConsensusTimeoutMs) * time.Millisecond,
		ConsensusThreshold: genesisRoot.Consensus.QuorumThreshold,
		RootTrustBase:      rootTrustBase,
		HashAlgorithm:      gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm),
	}
	return conf
}

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(host *network.Peer, genesisRoot *genesis.GenesisRootRecord,
	partitionStore PartitionStore, stateStore StateStore, signer crypto.Signer, net RootNet) (*ConsensusManager, error) {
	// Sanity checks
	if genesisRoot == nil {
		return nil, errors.New("cannot start distributed consensus, genesis root record is nil")
	}
	// load configuration
	conf := loadConf(genesisRoot)
	if net == nil {
		return nil, errors.New("network is nil")
	}
	selfID := host.ID().String()
	log.SetContext(log.KeyNodeID, selfID)
	logger.Debug("Starting consensus manager")
	safetyModule, err := NewSafetyModule(signer)
	if err != nil {
		return nil, err
	}
	lastState, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	l, err := leader.NewRotatingLeader(host, 1)
	if err != nil {
		return nil, err
	}
	rootVerifier, err := NewRootClusterVerifier(conf.RootTrustBase, conf.ConsensusThreshold)
	if err != nil {
		return nil, err
	}
	pipeline := NewRoundPipeline(conf.HashAlgorithm, lastState, partitionStore)
	if err != nil {
		return nil, err
	}

	consensusManager := &ConsensusManager{
		certReqCh:      make(chan consensus.IRChangeRequest),
		certResultCh:   make(chan certificates.UnicityCertificate),
		config:         conf,
		peer:           host,
		timers:         timer.NewTimers(),
		net:            net,
		pacemaker:      NewPacemaker(lastState.LatestRound, conf.LocalTimeoutMs, conf.BlockRateMs),
		leaderSelector: l,
		rootVerifier:   rootVerifier,
		irReqBuffer:    NewIrReqBuffer(),
		safety:         safetyModule,
		roundPipeline:  pipeline,
		partitions:     partitionStore,
		stateStore:     stateStore,
		waitPropose:    false,
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	// start
	consensusManager.start()
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) start() {
	// Start timers and network processing
	x.timers.Start(localTimeoutID, x.config.LocalTimeoutMs)
	// Am I the next leader?
	currentRound := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(currentRound) == x.peer.ID() {
		x.waitPropose = true
		x.timers.Start(blockRateID, x.config.BlockRateMs)
	}
	go x.loop()
}

func (x *ConsensusManager) Stop() {
	x.timers.WaitClose()
	x.ctxCancel()
}

func (x *ConsensusManager) loop() {
	for {
		select {
		case <-x.ctx.Done():
			logger.Info("Exiting distributed consensus manager main loop")
			return
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				logger.Warning("Root network received channel closed, exiting distributed consensus main loop")
				return
			}
			if msg.Message == nil {
				logger.Warning("Root network received message is nil")
				continue
			}
			switch msg.Protocol {
			case network.ProtocolRootIrChangeReq:
				req, correctType := msg.Message.(*atomic_broadcast.IRChangeReqMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("IR Change Request from %v", msg.From), req)
				x.onIRChange(req)
			case network.ProtocolRootProposal:
				req, correctType := msg.Message.(*atomic_broadcast.ProposalMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Proposal from %v", msg.From), req)
				x.onProposalMsg(req)
			case network.ProtocolRootVote:
				req, correctType := msg.Message.(*atomic_broadcast.VoteMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Vote from %v", msg.From), req)
				x.onVoteMsg(req)
			case network.ProtocolRootTimeout:
				req, correctType := msg.Message.(*atomic_broadcast.TimeoutMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Timeout vote from %v", msg.From), req)
				x.onTimeoutMsg(req)
				// Todo: AB-320 add handling
				/*
					case network.ProtocolRootStateReq:
						req, correctType := msg.Message.(*atomic_broadcast.StateRequestMsg)
						if !correctType {
							logger.Warning("Type %T not supported", msg.Message)
							continue
						}
						util.WriteDebugJsonLog(logger, fmt.Sprintf("Received recovery request from %v", msg.From), req)
						// Send state, with proof (signed by other validators)
					case network.ProtocolRootStateResp:
						req, correctType := msg.Message.(*atomic_broadcast.StateReplyMsg)
						if !correctType {
							logger.Warning("Type %T not supported", msg.Message)
							continue
						}
						util.WriteDebugJsonLog(logger, fmt.Sprintf("Received recovery response from %v", msg.From), req)
						// Verify and store
				*/
			default:
				logger.Warning("Unknown protocol req %s from %v", msg.Protocol, msg.From)
			}
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("certification channel closed, exiting distributed consensus main loop %v")
				return
			}
			x.onPartitionIRChangeReq(&req)
		// handle timeouts
		case nt, ok := <-x.timers.C:
			if !ok {
				logger.Warning("Timers channel closed, exiting main loop")
				return
			}
			if nt == nil {
				logger.Warning("Root timer channel received nil timer")
				continue
			}
			timerID := nt.Name()
			switch {
			case timerID == localTimeoutID:
				x.onLocalTimeout()
			case timerID == blockRateID:
				// throttling, make a proposal
				x.processNewRoundEvent()
			default:
				logger.Warning("Unknown timer %v", timerID)
			}
		}
	}
}

func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) {
	logger.Debug("Round %v, IR change request from partition", x.pacemaker.GetCurrentRound())
	reason := atomic_broadcast.IRChangeReqMsg_QUORUM
	if req.Reason == consensus.QuorumNotPossible {
		reason = atomic_broadcast.IRChangeReqMsg_QUORUM_NOT_POSSIBLE
	}
	irReq := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: req.SystemIdentifier.Bytes(),
		CertReason:       reason,
		Requests:         req.Requests}
	// are we the next leader or leader in current round waiting/throttling to send proposal
	nextRound := x.pacemaker.GetCurrentRound() + 1
	nextLeader := x.leaderSelector.GetLeaderForRound(nextRound)
	if x.leaderSelector.GetLeaderForRound(nextRound) == x.peer.ID() || x.waitPropose == true {
		// store the request in local buffer
		x.onIRChange(irReq)
		return
	}
	// route the IR change to the next root chain leader
	logger.Info("Round %v forwarding IR change request to next leader in round %v - %v",
		x.pacemaker.GetCurrentRound(), nextRound, nextLeader.String())
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irReq,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("Failed to forward IR Change request: %v", err)
	}
}

// onLocalTimeout handle timeouts
func (x *ConsensusManager) onLocalTimeout() {
	// always restart timer, time might be adjusted in case
	defer x.timers.Restart(localTimeoutID)
	logger.Info("Round %v local timeout", x.pacemaker.GetCurrentRound())
	// Has the validator voted in this round, if true send the same vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = atomic_broadcast.NewTimeoutMsg(atomic_broadcast.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.roundPipeline.GetHighQc()), x.peer.ID().String())
		// sign
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("Local timeout error %v", err)
			return
		}
		// Record vote
		x.pacemaker.SetTimeoutVote(timeoutVoteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	receivers := make([]peer.ID, len(x.leaderSelector.GetRootNodes()))
	for i, validator := range x.leaderSelector.GetRootNodes() {
		id, _ := validator.GetID()
		receivers[i] = id
	}
	logger.Info("Broadcasting timeout vote")
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootTimeout,
			Message:  timeoutVoteMsg,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send vote message: %v", err)
	}
}

// onIRChange handles IR change request from other root nodes
func (x *ConsensusManager) onIRChange(irChange *atomic_broadcast.IRChangeReqMsg) {
	// Am I the next leader or current leader and have not yet proposed? If not, ignore.
	// todo: AB-549 what if this is received out of order?
	// todo: AB-547 I am leader now, but have not yet proposed -> should still accept requests (throttling)
	if x.waitPropose == false {
		nextRound := x.pacemaker.GetCurrentRound() + 1
		if x.leaderSelector.GetLeaderForRound(nextRound) != x.peer.ID() {
			logger.Warning("Validator %v is not leader in next round %v, IR change req ignored",
				x.peer.ID().String(), nextRound)
			return
		}
	}
	logger.Info("Round %v IR change request received", x.pacemaker.GetCurrentRound())
	// validate incoming request
	sysID := p.SystemIdentifier(irChange.SystemIdentifier)
	partitionInfo, err := x.partitions.Info(sysID)
	if err != nil {
		logger.Warning("IR change error, failed to get total nods for partition %X, error: %v", sysID.Bytes(), err)
		return
	}
	state, err := x.stateStore.Get()
	if err != nil {
		logger.Warning("IR change request ignored, failed to read current state: %w", err)
		return
	}
	luc, f := state.Certificates[sysID]
	if !f {
		logger.Warning("IR change request ignored, no last state for system id %X", sysID.Bytes())
		return
	}
	// calculate LUC age using rounds and min block rate
	lucAgeInRounds := time.Duration(x.pacemaker.GetCurrentRound()-luc.UnicitySeal.RootRoundInfo.RoundNumber) * x.config.BlockRateMs
	inputRecord, err := irChange.Verify(partitionInfo, luc, lucAgeInRounds)
	if err != nil {
		logger.Warning("Invalid IR change request error: %v", err)
		return
	}
	// Check partition change already in pipeline
	if x.roundPipeline.IsChangeInPipeline(sysID) {
		logger.Warning("Invalid IR change request partition %X: change in pipeline",
			sysID.Bytes())
		return
	}
	// Buffer and wait for opportunity to make the next proposal
	if err := x.irReqBuffer.Add(IRChange{InputRecord: inputRecord, Reason: irChange.CertReason, Msg: irChange}); err != nil {
		logger.Warning("IR change request from partition %X error: %w", sysID.Bytes(), err)
		return
	}
}

// onVoteMsg handle votes and timeout votes
func (x *ConsensusManager) onVoteMsg(vote *atomic_broadcast.VoteMsg) {
	// verify signature on vote
	err := vote.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Vote verify failed: %v", err)
	}
	// SyncState
	if x.checkRecoveryNeeded(vote.HighQc) {
		logger.Error("Recovery not yet implemented")
		// todo: AB-320 try to recover
		return
	}
	logger.Info("Round %v vote from %v", x.pacemaker.GetCurrentRound(), vote.Author)
	// Was the proposal received? If not, should we recover it? or do we accept that this round will end up as timeout
	round := vote.VoteInfo.RoundNumber
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	nextRound := round + 1
	// verify that the validator is correct leader in next round
	if x.leaderSelector.GetLeaderForRound(nextRound) != x.peer.ID() {
		// this might also be a stale vote, since when we have quorum the round is advanced and the node becomes
		// the new leader in the current view/round
		logger.Warning("Received vote, validator is not leader in next round %v, vote ignored", nextRound)
		return
	}
	// Store vote, check for QC
	qc := x.pacemaker.RegisterVote(vote, x.rootVerifier)
	if qc != nil {
		logger.Info("Round %v quorum achieved", vote.VoteInfo.RoundNumber)
		// advance round (only done once)
		// NB! it must be able to
		x.processCertificateQC(qc)
		// since the root chain must not run faster than block-rate, calculate
		// time from last proposal and see if we need to wait
		slowDownTime := x.pacemaker.CalcTimeTilNextProposal()
		if slowDownTime > 0 {
			logger.Info("Round %v node %v wait %v before proposing",
				x.pacemaker.GetCurrentRound(), x.peer.ID().String(), slowDownTime)
			x.waitPropose = true
			x.timers.Start(blockRateID, slowDownTime)
			return
		}
		// trigger new round immediately
		x.processNewRoundEvent()
	}
}

func (x *ConsensusManager) onTimeoutMsg(vote *atomic_broadcast.TimeoutMsg) {
	// verify signature on vote
	err := vote.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Timeout vote verify failed: %v", err)
	}
	// Author voted timeout, proceed
	logger.Info("Round %v timout vote from %v",
		vote.Timeout.Round, vote.Author)
	// SyncState
	if x.checkRecoveryNeeded(vote.Timeout.HighQc) {
		logger.Error("Recovery not yet implemented")
		// todo: AB-320 try to recover
	}
	tc := x.pacemaker.RegisterTimeoutVote(vote, x.rootVerifier)
	if tc != nil {
		logger.Info("Round %v timeout quorum achieved", vote.Timeout.Round)
		x.processTC(tc)
	}
}

func (x *ConsensusManager) checkRecoveryNeeded(qc *atomic_broadcast.QuorumCert) bool {
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, x.roundPipeline.GetHighQc().VoteInfo.CurrentRootHash) {
		logger.Warning("QC round %v state is different, recover state", qc.VoteInfo.RoundNumber)
		return true
	}
	return false
}

func (x *ConsensusManager) VerifyProposalPayload(payload *atomic_broadcast.Payload) (map[p.SystemIdentifier]*certificates.InputRecord, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	committedState, err := x.stateStore.Get()
	if err != nil {
		return nil, err
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest
	// Remember all partitions that have changes in the current proposal and apply changes
	changes := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for _, irChReq := range payload.Requests {
		systemID := p.SystemIdentifier(irChReq.SystemIdentifier)
		// verify certification Request
		luc, found := committedState.Certificates[systemID]
		if !found {
			return nil, errors.Errorf("invalid payload: partition %X state is missing", systemID)
		}
		// Find if the SystemIdentifier is known by partition store
		partitionInfo, err := x.partitions.Info(systemID)
		if err != nil {
			return nil, fmt.Errorf("invalid payload: unknown partition %X", systemID.Bytes())
		}
		lucAgeInRounds := time.Duration(x.pacemaker.GetCurrentRound()-luc.UnicitySeal.RootRoundInfo.RoundNumber) * x.config.BlockRateMs
		// verify request
		inputRecord, err := irChReq.Verify(partitionInfo, luc, lucAgeInRounds)
		if err != nil {
			return nil, fmt.Errorf("invalid payload: partition %X certification request verifiaction failed %w", systemID.Bytes(), err)
		}
		changes[systemID] = inputRecord
	}
	return changes, nil
}

func (x *ConsensusManager) onProposalMsg(proposal *atomic_broadcast.ProposalMsg) {
	// verify signature on proposal (does not verify partition request signatures)
	err := proposal.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Invalid Proposal message, verify failed: %v", err)
	}
	logger.Info("Received proposal message from %v", proposal.Block.Author)
	// Is from valid leader
	if x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String() != proposal.Block.Author {
		logger.Warning("Proposal author %v is not a valid leader for round %v, ignoring proposal",
			proposal.Block.Author, proposal.Block.Round)
		return
	}
	// timestamp proposal received, used to make sure root chain is not running faster than block time
	x.pacemaker.RegisterProposal()
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processCertificateQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	// Check in sync with other root nodes
	if x.checkRecoveryNeeded(proposal.Block.Qc) {
		logger.Error("Recovery not yet implemented")
		// todo: AB-320 try to recover
		return
	}
	// execute proposed payload
	changes, err := x.VerifyProposalPayload(proposal.Block.Payload)
	// execute proposal
	execStateId, err := x.roundPipeline.Add(x.pacemaker.GetCurrentRound(), changes)
	if err != nil {
		logger.Warning("Failed to execute proposal: %v", err.Error())
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, x.peer.ID().String(), x.pacemaker.LastRoundTC())
	if err != nil {
		logger.Warning("Failed to sign vote, vote not sent: %v", err.Error())
		return
	}
	// Add high Qc for state synchronization
	voteMsg.HighQc = x.roundPipeline.GetHighQc()

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	logger.Info("Sending vote to next leader %v", nextLeader.String())
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("Failed to send vote message: %v", err)
	}
}

func (x *ConsensusManager) processCertificateQC(qc *atomic_broadcast.QuorumCert) {
	if qc == nil {
		return
	}
	committedState := x.roundPipeline.Update(qc)
	if committedState != nil {
		if err := x.stateStore.Save(committedState); err != nil {
			logger.Warning("Failed persist new root chain state: %w", err)
		}
		for _, cert := range committedState.Certificates {
			x.certResultCh <- *cert
		}
	}
	x.pacemaker.AdvanceRoundQC(qc)
	// progress is made, restart timeout (move to pacemaker)
	x.timers.Restart(localTimeoutID)
}

func (x *ConsensusManager) processTC(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	// restore state pipeline to last persisted state
	lastState, err := x.stateStore.Get()
	if err != nil {
		logger.Error("Unable to restore last persisted state, %w", err)
		// todo: this will probably never happen, but find a way to improve this
		// keep a local copy of last persisted state at hand all the time?
		return
	}
	x.roundPipeline.Reset(lastState)
	x.pacemaker.AdvanceRoundTC(tc)
	// as this is TC case, there is no need to throttle, we are already behind
	// start new round and issue a proposal if leader
	x.processNewRoundEvent()
}

func (x *ConsensusManager) generateTimeoutRequests() ([]*atomic_broadcast.IRChangeReqMsg, error) {
	// get latest committed state
	state, err := x.stateStore.Get()
	if err != nil {
		return nil, err
	}
	timeoutRequests := make([]*atomic_broadcast.IRChangeReqMsg, 0, len(state.Certificates))
	for id, cert := range state.Certificates {
		// if there is a change in progress or in request buffer (prefer progress to timeout) then skip
		if x.roundPipeline.IsChangeInPipeline(id) || x.irReqBuffer.IsChangeInBuffer(id) {
			continue
		}
		partInfo, err := x.partitions.Info(id)
		if err != nil {
			return nil, err
		}
		if time.Duration(x.pacemaker.GetCurrentRound()-cert.UnicitySeal.RootRoundInfo.RoundNumber)*x.config.BlockRateMs >=
			time.Duration(partInfo.SystemDescription.T2Timeout)*time.Millisecond {
			logger.Info("Round %v request partition %X T2 timeout", x.pacemaker.GetCurrentRound(), id.Bytes())
			timeoutReq := &atomic_broadcast.IRChangeReqMsg{
				SystemIdentifier: id.Bytes(),
				CertReason:       atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT,
			}
			timeoutRequests = append(timeoutRequests, timeoutReq)
		}
	}
	return timeoutRequests, nil
}

func (x *ConsensusManager) processNewRoundEvent() {
	logger.Info("Round %v start", x.pacemaker.GetCurrentRound())
	// to counteract throttling (find a better solution)
	x.waitPropose = false
	x.timers.Restart(localTimeoutID)
	round := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(round) != x.peer.ID() {
		logger.Warning("Round %v current node is not the leader, await proposal", round)
		return
	}
	logger.Info("Round %v node %v is leader in round", x.pacemaker.GetCurrentRound(), x.peer.ID().String())
	// create proposal message, generate payload of IR change requests
	timeoutReqs, err := x.generateTimeoutRequests()
	if err != nil {
		logger.Error("Round %v unable to generate timeout requests: %v", err)
	}
	payload := x.irReqBuffer.GeneratePayload()
	payload.Requests = append(payload.GetRequests(), timeoutReqs...)
	proposalMsg := &atomic_broadcast.ProposalMsg{
		Block: &atomic_broadcast.BlockData{
			Author:    x.peer.ID().String(),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   payload,
			Qc:        x.roundPipeline.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err := x.safety.SignProposal(proposalMsg); err != nil {
		logger.Warning("Failed to send proposal message, message signing failed: %v", err)
	}
	// broadcast proposal message (also to self)
	receivers := make([]peer.ID, len(x.leaderSelector.GetRootNodes()))
	for i, validator := range x.leaderSelector.GetRootNodes() {
		id, _ := validator.GetID()
		receivers[i] = id
	}
	logger.Info("Broadcasting proposal msg")
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send proposal message, network error: %v", err)
	}
}
