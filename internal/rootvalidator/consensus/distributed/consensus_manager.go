package distributed

import (
	"bytes"
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed/leader"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed/storage"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
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

	ConsensusManager struct {
		ctx            context.Context
		ctxCancel      context.CancelFunc
		certReqCh      chan consensus.IRChangeRequest
		certResultCh   chan certificates.UnicityCertificate
		params         *consensus.Parameters
		peer           *network.Peer
		timers         *timer.Timers
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		trustBase      *RootTrustBase
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		datastore      *storage.Storage
		blockStore     *storage.BlockStore
		partitions     PartitionStore
		irReqVerifier  *IRChangeReqVerifier
		t2Timeouts     *PartitionTimeoutGenerator
		waitPropose    bool
		voteBuffer     []*atomic_broadcast.VoteMsg
	}
)

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(host *network.Peer, rg *genesis.RootGenesis,
	partitionStore PartitionStore, net RootNet, signer crypto.Signer, opts ...consensus.Option) (*ConsensusManager, error) {
	// Sanity checks
	if rg == nil {
		return nil, errors.New("cannot start distributed consensus, genesis root record is nil")
	}
	if net == nil {
		return nil, errors.New("network is nil")
	}
	// load options
	optional := consensus.LoadConf(opts)
	// load consensus configuration from genesis
	cParams := consensus.NewConsensusParams(rg.Root)
	// init storage
	dataStore, err := storage.New(optional.StoragePath)
	bStore, err := storage.NewBlockStore(cParams.HashAlgorithm, rg.Partitions, dataStore)
	if err != nil {
		return nil, fmt.Errorf("consensus block storage init failed, %w", err)
	}
	reqVerifier, err := NewIRChangeReqVerifier(cParams, partitionStore, bStore)
	t2TimeoutGen, err := NewLucBasedT2TimeoutGenerator(cParams, partitionStore, bStore)
	if err != nil {
		return nil, fmt.Errorf("block verifier construct error, %w", err)
	}
	hqcRound := bStore.GetHighQc().VoteInfo.RoundNumber
	safetyModule, err := NewSafetyModule(host.ID().String(), signer, dataStore.GetRootDB())
	if err != nil {
		return nil, err
	}
	l, err := leader.NewRotatingLeader(host, 1)
	if err != nil {
		return nil, fmt.Errorf("consensus leader election init failed, %w", err)
	}
	tb, err := NewRootTrustBaseFromGenesis(rg.Root)
	if err != nil {
		return nil, fmt.Errorf("consensus root trust base init failed, %w", err)
	}
	consensusManager := &ConsensusManager{
		certReqCh:      make(chan consensus.IRChangeRequest),
		certResultCh:   make(chan certificates.UnicityCertificate),
		params:         cParams,
		peer:           host,
		timers:         timer.NewTimers(),
		net:            net,
		pacemaker:      NewPacemaker(hqcRound, cParams.LocalTimeoutMs, cParams.BlockRateMs),
		leaderSelector: l,
		trustBase:      tb,
		irReqBuffer:    NewIrReqBuffer(),
		safety:         safetyModule,
		blockStore:     bStore,
		partitions:     partitionStore,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
		waitPropose:    false,
		voteBuffer:     []*atomic_broadcast.VoteMsg{},
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

func (x *ConsensusManager) GetLatestUnicityCertificate(id p.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	ucs := x.blockStore.GetCertificates()
	luc, f := ucs[id]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

func (x *ConsensusManager) start() {
	// Start timers and network processing
	x.timers.Start(localTimeoutID, x.params.LocalTimeoutMs)
	// Am I the leader?
	currentRound := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(currentRound) == x.peer.ID() {
		logger.Info("%v round %v root node started as leader, waiting to propose", x.peer.LogID(), currentRound)
		// on start wait a bit before making a proposal
		x.waitPropose = true
		x.timers.Start(blockRateID, x.params.BlockRateMs)
	} else {
		logger.Info("%v round %v root node started, waiting for proposal from leader %v",
			x.peer.LogID(), currentRound, x.leaderSelector.GetLeaderForRound(currentRound).String())
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
			logger.Info("%v exiting distributed consensus manager main loop", x.peer.LogID())
			return
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				logger.Warning("%v root network received channel closed, exiting distributed consensus main loop",
					x.peer.LogID())
				return
			}
			if msg.Message == nil {
				logger.Warning("%v root network received message is nil", x.peer.LogID())
				continue
			}
			switch msg.Protocol {
			case network.ProtocolRootIrChangeReq:
				req, correctType := msg.Message.(*atomic_broadcast.IRChangeReqMsg)
				if !correctType {
					logger.Warning("%v type %T not supported", x.peer.LogID(), msg.Message)
					continue
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("IR Change Request from %v", msg.From), req)
				x.onIRChange(req)
			case network.ProtocolRootProposal:
				req, correctType := msg.Message.(*atomic_broadcast.ProposalMsg)
				if !correctType {
					logger.Warning("%v type %T not supported", x.peer.LogID(), msg.Message)
					continue
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Proposal from %v", msg.From), req)
				x.onProposalMsg(req)
			case network.ProtocolRootVote:
				req, correctType := msg.Message.(*atomic_broadcast.VoteMsg)
				if !correctType {
					logger.Warning("%v type %T not supported", x.peer.LogID(), msg.Message)
					continue
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Vote from %v", msg.From), req)
				x.onVoteMsg(req)
			case network.ProtocolRootTimeout:
				req, correctType := msg.Message.(*atomic_broadcast.TimeoutMsg)
				if !correctType {
					logger.Warning("%v type %T not supported", x.peer.LogID(), msg.Message)
					continue
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Timeout vote from %v", msg.From), req)
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
				logger.Warning("%v unknown protocol req %s from %v", x.peer.LogID(), msg.Protocol, msg.From)
			}
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("%v certification channel closed, exiting distributed consensus main loop", x.peer.LogID())
				return
			}
			x.onPartitionIRChangeReq(&req)
		// handle timeouts
		case nt, ok := <-x.timers.C:
			if !ok {
				logger.Warning("%v timers channel closed, exiting main loop", x.peer.LogID())
				return
			}
			if nt == nil {
				logger.Warning("%v root timer channel received nil timer", x.peer.LogID())
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
				logger.Warning("%v unknown timer %v", x.peer.LogID(), timerID)
			}
		}
	}
}

func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) {
	logger.Trace("%v round %v, IR change request from partition",
		x.peer.LogID(), x.pacemaker.GetCurrentRound())
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
	logger.Trace("%v round %v forwarding IR change request to next leader in round %v - %v",
		x.peer.LogID(), x.pacemaker.GetCurrentRound(), nextRound, nextLeader.String())
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irReq,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to forward IR Change request: %v", x.peer.LogID(), err)
	}
}

// onLocalTimeout handle timeouts
func (x *ConsensusManager) onLocalTimeout() {
	// always restart timer, time might be adjusted in case a new round is started
	// to account for throttling
	defer x.timers.Restart(localTimeoutID)
	logger.Info("%v round %v local timeout", x.peer.LogID(), x.pacemaker.GetCurrentRound())
	// Has the validator voted in this round, if true send the same vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = atomic_broadcast.NewTimeoutMsg(atomic_broadcast.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc()), x.peer.ID().String())
		// sign
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("%v local timeout error, %v", x.peer.LogID(), err)
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
	logger.Trace("%v round %v broadcasting timeout vote", x.peer.LogID(), x.pacemaker.GetCurrentRound())
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootTimeout,
			Message:  timeoutVoteMsg,
		}, receivers)
	if err != nil {
		logger.Warning("%v failed to send vote message: %v", x.peer.LogID(), err)
	}
}

// onIRChange handles IR change request from other root nodes
func (x *ConsensusManager) onIRChange(irChange *atomic_broadcast.IRChangeReqMsg) {
	// Am I the next leader or current leader and have not yet proposed? If not, ignore.
	// todo: AB-549 what if I am no longer the leader, should this be forwarded again to the next leader?
	if x.waitPropose == false {
		// forward to the next round leader
		nextRound := x.pacemaker.GetCurrentRound() + 1
		if x.leaderSelector.GetLeaderForRound(nextRound) != x.peer.ID() {
			logger.Warning("%v is not leader in next round %v, IR change req ignored",
				x.peer.LogID(), nextRound)
			return
		}
	}
	logger.Trace("%v round %v IR change request received", x.peer.LogID(), x.pacemaker.GetCurrentRound())
	// Verify and buffer and wait for opportunity to make the next proposal
	if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChange, x.irReqVerifier); err != nil {
		logger.Warning("%v IR change request from partition %X error: %v", x.peer.LogID(), irChange.SystemIdentifier, err)
		return
	}
}

// onVoteMsg handle votes and timeout votes
func (x *ConsensusManager) onVoteMsg(vote *atomic_broadcast.VoteMsg) {
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v vote verify failed: %v", x.peer.LogID(), err)
	}
	if vote.VoteInfo.RoundNumber < x.pacemaker.GetCurrentRound() {
		logger.Warning("%v round %v stale vote, validator %v is behind vote for round %v, ignored",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.Author, vote.VoteInfo.RoundNumber)
		return
	}
	logger.Trace("%v round %v received vote for round %v from %v",
		x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber, vote.Author)
	// if a vote is received for the next round and this node is going to be the leader in
	// the round after this, then buffer vote, it was just received before the proposal
	if vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound()+1 &&
		x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound()+2) == x.peer.ID() {
		// vote received before proposal, buffer
		logger.Debug("%v round %v received vote for round %v before proposal, buffering vote",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		x.voteBuffer = append(x.voteBuffer, vote)
		return
	}
	round := vote.VoteInfo.RoundNumber
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	nextRound := round + 1
	// verify that the validator is correct leader in next round
	if x.leaderSelector.GetLeaderForRound(nextRound) != x.peer.ID() {
		// this might also be a stale vote, since when we have quorum the round is advanced and the node becomes
		// the new leader in the current view/round
		logger.Warning("%v received vote, validator is not leader in next round %v, vote ignored",
			x.peer.LogID(), nextRound)
		return
	}
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.HighQc) {
		logger.Error("%v vote handling, recovery not yet implemented", x.peer.LogID())
		// todo: AB-320 try to recover
		return
	}
	// Store vote, check for QC
	qc := x.pacemaker.RegisterVote(vote, x.trustBase)
	if qc == nil {
		logger.Trace("%v round %v processed vote for round %v, no quorum yet",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		return
	}
	logger.Debug("%v round %v quorum achieved", x.peer.LogID(), vote.VoteInfo.RoundNumber)
	// since the root chain must not run faster than block-rate, calculate
	// time from last proposal and see if we need to wait
	slowDownTime := x.pacemaker.CalcTimeTilNextProposal(x.pacemaker.GetCurrentRound() + 1)
	// advance view/round on QC
	x.processCertificateQC(qc)
	if slowDownTime > 0 {
		logger.Trace("%v round %v node %v wait %v before proposing",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), x.peer.ID().String(), slowDownTime)
		x.waitPropose = true
		x.timers.Start(blockRateID, slowDownTime)
		return
	}
	// trigger new round immediately
	x.processNewRoundEvent()
}

func (x *ConsensusManager) onTimeoutMsg(vote *atomic_broadcast.TimeoutMsg) {
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v timeout vote verify failed: %v", x.peer.LogID(), err)
	}
	// stale drop
	if vote.Timeout.Round < x.pacemaker.GetCurrentRound() {
		logger.Trace("%v round %v stale timeout vote, validator %v voted for timeout in round %v, ignored",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.Author, vote.Timeout.Round)
		return
	}
	// Node voted timeout, proceed
	logger.Trace("%v round %v received timout vote for round %v from %v",
		x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round, vote.Author)
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.Timeout.HighQc) {
		logger.Error("%v timeout vote, recovery not yet implemented", x.peer.LogID())
		// todo: AB-320 try to recover
	}
	tc := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if tc == nil {
		logger.Trace("%v round %v processed timeout vote for round %v, no quorum yet",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round)
		return
	}
	logger.Debug("%v round %v timeout quorum achieved", x.peer.LogID(), vote.Timeout.Round)
	// process timeout certificate to advance to next the view/round
	x.processTC(tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.peer.ID() {
		x.processNewRoundEvent()
	} else {
		logger.Trace("%v round %v, new leader is %v, waiting for proposal",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), l.String())
	}
}

func (x *ConsensusManager) checkRecoveryNeeded(qc *atomic_broadcast.QuorumCert) bool {
	// Get block and check if we have the same state
	rootHash, err := x.blockStore.GetBlockRootHash(qc.VoteInfo.RoundNumber)
	if err != nil {
		logger.Warning("%v round %v unknown block in round %v, recover",
			x.peer.LogID(), qc.VoteInfo.RoundNumber)
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, rootHash) {
		logger.Warning("%v round %v state is different expected %X, local %X, recover state",
			x.peer.LogID(), qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, rootHash)
		return true
	}
	return false
}

func (x *ConsensusManager) onProposalMsg(proposal *atomic_broadcast.ProposalMsg) {
	// verify signature on proposal (does not verify partition request signatures)
	err := proposal.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v invalid Proposal message, verify failed: %v", x.peer.LogID(), err)
	}
	// stale drop
	if proposal.Block.Round < x.pacemaker.GetCurrentRound() {
		logger.Debug("%v round %v stale proposal, validator %v is behind and proposed round %v, ignored",
			x.peer.LogID(), x.pacemaker.GetCurrentRound(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	logger.Trace("%v round %v received proposal message from %v",
		x.peer.LogID(), x.pacemaker.GetCurrentRound(), proposal.Block.Author)
	// Is from valid leader
	if x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String() != proposal.Block.Author {
		logger.Warning("%v proposal author %v is not a valid leader for round %v, ignoring proposal",
			x.peer.LogID(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	// Check current state against new QC
	if x.checkRecoveryNeeded(proposal.Block.Qc) {
		logger.Error("%v Proposal handling, recovery not yet implemented", x.peer.LogID())
		// todo: AB-320 try to recover
		return
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processCertificateQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	// execute proposed payload
	execStateId, err := x.blockStore.Add(proposal.Block, x.irReqVerifier)
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to execute proposal: %v", x.peer.LogID(), err.Error())
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC())
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to sign vote, vote not sent: %v", x.peer.LogID(), err.Error())
		return
	}

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	logger.Trace("%v sending vote to next leader %v", x.peer.LogID(), nextLeader.String())
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to send vote message: %v", x.peer.LogID(), err)
	}
	// process vote buffer
	if len(x.voteBuffer) > 0 {
		logger.Debug("Handling %v buffered vote messages", len(x.voteBuffer))
		for _, v := range x.voteBuffer {
			x.onVoteMsg(v)
		}
		// clear
		x.voteBuffer = []*atomic_broadcast.VoteMsg{}
	}
}

func (x *ConsensusManager) processCertificateQC(qc *atomic_broadcast.QuorumCert) {
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		// todo: recovery
		logger.Warning("%v round %v process qc error %v, try to  recover", x.peer.LogID(), x.pacemaker.GetCurrentRound(), err)
		return
	}
	for _, uc := range certs {
		x.certResultCh <- *uc
	}
	x.pacemaker.AdvanceRoundQC(qc)
	// progress is made, restart timeout (move to pacemaker)
	x.timers.Restart(localTimeoutID)
}

func (x *ConsensusManager) processTC(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		logger.Warning("%v failed to handle timeout certificate: %v", x.peer.LogID(), err)
	}
	x.pacemaker.AdvanceRoundTC(tc)
}

func (x *ConsensusManager) processNewRoundEvent() {
	// to counteract throttling (find a better solution)
	x.waitPropose = false
	x.timers.Restart(localTimeoutID)
	round := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(round) != x.peer.ID() {
		logger.Info("%v round %v new round start, not leader awaiting proposal", x.peer.LogID(), round)
		return
	}
	logger.Info("%v round %v new round start, node is leader", x.peer.LogID(), x.pacemaker.GetCurrentRound())
	proposalMsg := &atomic_broadcast.ProposalMsg{
		Block: &atomic_broadcast.BlockData{
			Author:    x.peer.ID().String(),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(x.pacemaker.GetCurrentRound(), x.t2Timeouts),
			Qc:        x.blockStore.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err := x.safety.SignProposal(proposalMsg); err != nil {
		logger.Warning("%v failed to send proposal message, message signing failed: %v", x.peer.LogID(), err)
	}
	// broadcast proposal message (also to self)
	receivers := make([]peer.ID, len(x.leaderSelector.GetRootNodes()))
	for i, validator := range x.leaderSelector.GetRootNodes() {
		id, _ := validator.GetID()
		receivers[i] = id
	}
	logger.Trace("%v broadcasting proposal msg", x.peer.LogID())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg}, receivers); err != nil {
		logger.Warning("%v failed to send proposal message, network error: %v", x.peer.LogID(), err)
	}
}
