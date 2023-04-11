package distributed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/distributed/leader"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/distributed/storage"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// local timeout
	blockRateID = "block-rate"
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

	ConsensusManager struct {
		ctx            context.Context
		ctxCancel      context.CancelFunc
		certReqCh      chan consensus.IRChangeRequest
		certResultCh   chan *certificates.UnicityCertificate
		params         *consensus.Parameters
		peer           *network.Peer
		localTimeout   *time.Ticker
		timers         *timer.Timers
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		trustBase      *RootTrustBase
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		datastore      *storage.Storage
		blockStore     *storage.BlockStore
		partitions     partitions.PartitionConfiguration
		irReqVerifier  *IRChangeReqVerifier
		t2Timeouts     *PartitionTimeoutGenerator
		waitPropose    bool
		voteBuffer     []*atomic_broadcast.VoteMsg
	}
)

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(host *network.Peer, rg *genesis.RootGenesis,
	partitionStore partitions.PartitionConfiguration, net RootNet, signer crypto.Signer, opts ...consensus.Option) (*ConsensusManager, error) {
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
		certResultCh:   make(chan *certificates.UnicityCertificate),
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
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan *certificates.UnicityCertificate {
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

func (x *ConsensusManager) Run(ctx context.Context) error {
	defer x.timers.WaitClose()
	// Start timers and network processing
	x.localTimeout = time.NewTicker(x.params.LocalTimeoutMs)
	// Am I the leader?
	currentRound := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(currentRound) == x.peer.ID() {
		logger.Info("%v round %v root node started as leader, waiting to propose", x.peer.String(), currentRound)
		// on start wait a bit before making a proposal
		x.waitPropose = true
		x.timers.Start(blockRateID, x.params.BlockRateMs)
	} else {
		logger.Info("%v round %v root node started, waiting for proposal from leader %v",
			x.peer.String(), currentRound, x.leaderSelector.GetLeaderForRound(currentRound).String())
	}
	return x.loop(ctx)
}

func (x *ConsensusManager) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("%v exiting distributed consensus manager main loop", x.peer.String())
			return ctx.Err()
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				logger.Warning("%v root network received channel closed, exiting distributed consensus main loop",
					x.peer.String())
				return fmt.Errorf("exit drc consensus, network channel closed")
			}
			if msg.Message == nil {
				logger.Warning("%v root network received message is nil", x.peer.String())
				continue
			}
			switch mt := msg.Message.(type) {
			case *atomic_broadcast.IRChangeReqMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("IR Change Request from %v", msg.From), mt)
				x.onIRChange(mt)
			case *atomic_broadcast.ProposalMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Proposal from %v", msg.From), mt)
				x.onProposalMsg(mt)
			case *atomic_broadcast.VoteMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Vote from %v", msg.From), mt)
				x.onVoteMsg(mt)
			case *atomic_broadcast.TimeoutMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Timeout vote from %v", msg.From), mt)
				x.onTimeoutMsg(mt)
			case *atomic_broadcast.GetCertificates:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Recovery certificates request from %v", msg.From), mt)
				x.onCertificateReq(mt)
			case *atomic_broadcast.CertificatesMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Recovery certificates response from %v", msg.From), mt)
				x.onCertificateResp(mt.Certificates)
			case *atomic_broadcast.GetStateMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Recovery state request from %v", msg.From), mt)
				x.onStateReq(mt)
			case *atomic_broadcast.StateMsg:
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Received recovery response from %v", msg.From), mt)
				x.onStateResponse(mt)
			default:
				logger.Warning("%v unknown protocol req %s %T from %v", x.peer.String(), msg.Protocol, mt, msg.From)
			}
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("%v certification channel closed, exiting distributed consensus main loop", x.peer.String())
				return fmt.Errorf("exit drc consensus, certification channel closed")
			}
			x.onPartitionIRChangeReq(&req)
		case <-x.localTimeout.C:
			x.onLocalTimeout()

		// handle timeouts
		case nt, ok := <-x.timers.C:
			if !ok {
				logger.Warning("%v timers channel closed, exiting main loop", x.peer.String())
				return fmt.Errorf("exit drc consensus, timer channel closed")
			}
			if nt == nil {
				logger.Warning("%v root timer channel received nil timer", x.peer.String())
				continue
			}
			x.processNewRoundEvent()
		}
	}
}

// onLocalTimeout handle local timeout, either no proposal is received or voting does not
// reach consensus. Triggers timeout voting.
func (x *ConsensusManager) onLocalTimeout() {
	// always restart timer, time might be adjusted in case a new round is started
	// to account for throttling
	logger.Info("%v round %v local timeout", x.peer.String(), x.pacemaker.GetCurrentRound())
	// Has the validator voted in this round, if true send the same vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = atomic_broadcast.NewTimeoutMsg(atomic_broadcast.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc()), x.peer.ID().String())
		// sign
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("%v local timeout error, %v", x.peer.String(), err)
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
	logger.Trace("%v round %v broadcasting timeout vote", x.peer.String(), x.pacemaker.GetCurrentRound())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootTimeout,
			Message:  timeoutVoteMsg,
		}, receivers); err != nil {
		logger.Warning("%v failed to forward ir change message: %v", x.peer.String(), err)
	}
}

// onPartitionIRChangeReq handle partition change requests. Received from go routine handling
// partition communication when either partition reaches consensus or cannot reach consensus.
func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) {
	logger.Debug("%v round %v, IR change request from partition",
		x.peer.String(), x.pacemaker.GetCurrentRound())
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
	logger.Debug("%v round %v forwarding IR change request to next leader in round %v - %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), nextRound, nextLeader.String())
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irReq,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to forward IR Change request: %v", x.peer.String(), err)
	}
}

// onIRChange handles IR change request from other root nodes
func (x *ConsensusManager) onIRChange(irChange *atomic_broadcast.IRChangeReqMsg) {
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// todo: if in recovery then forward to next?
	// if the node is either next leader or leader now, but has not yet proposed then buffer the request
	if (x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound()) == x.peer.ID() && x.waitPropose) ||
		(nextLeader == x.peer.ID()) {
		logger.Debug("%v round %v IR change request received", x.peer.String(), x.pacemaker.GetCurrentRound())
		// Verify and buffer and wait for opportunity to make the next proposal
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChange, x.irReqVerifier); err != nil {
			logger.Warning("%v IR change request from partition %X error: %v", x.peer.String(), irChange.SystemIdentifier, err)
		}
		return
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this a completely lots message or because of race we just proposed, try to forward again to the next leader
	logger.Warning("%v is not leader in next round %v, IR change req forwarded again %v",
		x.peer.String(), x.pacemaker.GetCurrentRound()+1, nextLeader.String())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irChange,
		}, []peer.ID{nextLeader}); err != nil {
		logger.Warning("%v failed to forward ir chang message: %v", x.peer.String(), err)
	}
}

// onVoteMsg handle votes messages from other root validators
func (x *ConsensusManager) onVoteMsg(vote *atomic_broadcast.VoteMsg) {
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v vote verify failed: %v", x.peer.String(), err)
	}
	if vote.VoteInfo.RoundNumber < x.pacemaker.GetCurrentRound() {
		logger.Warning("%v round %v stale vote, validator %v is behind vote for round %v, ignored",
			x.peer.String(), x.pacemaker.GetCurrentRound(), vote.Author, vote.VoteInfo.RoundNumber)
		return
	}
	logger.Trace("%v round %v received vote for round %v from %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber, vote.Author)
	// if a vote is received for the next round and this node is going to be the leader in
	// the round after this, then buffer vote, it was just received before the proposal
	if vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound()+1 &&
		x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound()+2) == x.peer.ID() {
		// vote received before proposal, buffer
		logger.Debug("%v round %v received vote for round %v before proposal, buffering vote",
			x.peer.String(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
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
			x.peer.String(), nextRound)
		return
	}
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.HighQc) {
		logger.Error("%v vote handling, recovery not yet implemented", x.peer.String())
		// todo: AB-320 try to recover
		return
	}
	// Store vote, check for QC
	qc := x.pacemaker.RegisterVote(vote, x.trustBase)
	if qc == nil {
		logger.Trace("%v round %v processed vote for round %v, no quorum yet",
			x.peer.String(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		return
	}
	logger.Debug("%v round %v quorum achieved", x.peer.String(), vote.VoteInfo.RoundNumber)
	// since the root chain must not run faster than block-rate, calculate
	// time from last proposal and see if we need to wait
	slowDownTime := x.pacemaker.CalcTimeTilNextProposal(x.pacemaker.GetCurrentRound() + 1)
	// advance view/round on QC
	x.processQC(qc)
	if slowDownTime > 0 {
		logger.Trace("%v round %v node %v wait %v before proposing",
			x.peer.String(), x.pacemaker.GetCurrentRound(), x.peer.ID().String(), slowDownTime)
		x.waitPropose = true
		x.timers.Start(blockRateID, slowDownTime)
		return
	}
	// trigger new round immediately
	x.processNewRoundEvent()
}

// onTimeoutMsg handles timeout vote messages from other root validators
// Timeout votes are broadcast to all nodes on local timeout and all validators try to assemble
// timeout certificate independently.
func (x *ConsensusManager) onTimeoutMsg(vote *atomic_broadcast.TimeoutMsg) {
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v timeout vote verify failed: %v", x.peer.String(), err)
	}
	// stale drop
	if vote.Timeout.Round < x.pacemaker.GetCurrentRound() {
		logger.Trace("%v round %v stale timeout vote, validator %v voted for timeout in round %v, ignored",
			x.peer.String(), x.pacemaker.GetCurrentRound(), vote.Author, vote.Timeout.Round)
		return
	}
	// Node voted timeout, proceed
	logger.Trace("%v round %v received timout vote for round %v from %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round, vote.Author)
	// SyncState, compare last handled QC
	if x.checkRecoveryNeeded(vote.Timeout.HighQc) {
		logger.Error("%v timeout vote, recovery not yet implemented", x.peer.String())
		// todo: AB-320 try to recover
	}
	tc := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if tc == nil {
		logger.Trace("%v round %v processed timeout vote for round %v, no quorum yet",
			x.peer.String(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round)
		return
	}
	logger.Debug("%v round %v timeout quorum achieved", x.peer.String(), vote.Timeout.Round)
	// process timeout certificate to advance to next the view/round
	x.processTC(tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.peer.ID() {
		x.processNewRoundEvent()
	} else {
		logger.Trace("%v round %v, new leader is %v, waiting for proposal",
			x.peer.String(), x.pacemaker.GetCurrentRound(), l.String())
	}
}

// checkRecoveryNeeded verify current state against received state and determine if the validator needs to
// recover or not. Basically either the state is different or validator is behind (has skipped some views/rounds)
func (x *ConsensusManager) checkRecoveryNeeded(qc *atomic_broadcast.QuorumCert) bool {
	// Get block and check if we have the same state
	rootHash, err := x.blockStore.GetBlockRootHash(qc.VoteInfo.RoundNumber)
	if err != nil {
		logger.Warning("%v round %v unknown block in round %v, recover",
			x.peer.String(), qc.VoteInfo.RoundNumber)
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, rootHash) {
		logger.Warning("%v round %v state is different expected %X, local %X, recover state",
			x.peer.String(), qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, rootHash)
		return true
	}
	return false
}

// onProposalMsg handles block proposal messages from other validators.
// Only a proposal made by the leader of this view/round shall be accepted and processed
func (x *ConsensusManager) onProposalMsg(proposal *atomic_broadcast.ProposalMsg) {
	// verify signature on proposal (does not verify partition request signatures)
	err := proposal.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		logger.Warning("%v invalid Proposal message, verify failed: %v", x.peer.String(), err)
	}
	// stale drop
	if proposal.Block.Round < x.pacemaker.GetCurrentRound() {
		logger.Debug("%v round %v stale proposal, validator %v is behind and proposed round %v, ignored",
			x.peer.String(), x.pacemaker.GetCurrentRound(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	logger.Trace("%v round %v received proposal message from %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), proposal.Block.Author)
	// Is from valid leader
	if x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String() != proposal.Block.Author {
		logger.Warning("%v proposal author %v is not a valid leader for round %v, ignoring proposal",
			x.peer.String(), proposal.Block.Author, proposal.Block.Round)
		return
	}
	// Check current state against new QC
	if x.checkRecoveryNeeded(proposal.Block.Qc) {
		logger.Error("%v Proposal handling, recovery not yet implemented", x.peer.String())
		// todo: AB-320 try to recover
		return
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	// execute proposed payload
	execStateId, err := x.blockStore.Add(proposal.Block, x.irReqVerifier)
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to execute proposal: %v", x.peer.String(), err.Error())
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC())
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		logger.Warning("%v failed to sign vote, vote not sent: %v", x.peer.String(), err.Error())
		return
	}

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	logger.Trace("%v sending vote to next leader %v", x.peer.String(), nextLeader.String())
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, []peer.ID{nextLeader})
	if err != nil {
		logger.Warning("%v failed to send vote message: %v", x.peer.String(), err)
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

// processQC - handles timeout certificate
func (x *ConsensusManager) processQC(qc *atomic_broadcast.QuorumCert) {
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		// todo: recovery
		logger.Warning("%v round %v process qc error %v, try to  recover", x.peer.String(), x.pacemaker.GetCurrentRound(), err)
		return
	}
	for _, uc := range certs {
		x.certResultCh <- uc
	}
	x.pacemaker.AdvanceRoundQC(qc)
	// progress is made, restart timeout (move to pacemaker)
	x.localTimeout.Reset(x.pacemaker.GetRoundTimeout())
}

// processTC - handles timeout certificate
func (x *ConsensusManager) processTC(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		logger.Warning("%v failed to handle timeout certificate: %v", x.peer.String(), err)
	}
	x.pacemaker.AdvanceRoundTC(tc)
}

// processNewRoundEvent handled new view event, is called when either QC or TC is reached locally and
// triggers a new round. If this node is the leader in the new view/round, then make a proposal otherwise
// wait for a proposal from a leader in this round/view
func (x *ConsensusManager) processNewRoundEvent() {
	// to counteract throttling (find a better solution)
	x.waitPropose = false
	x.localTimeout.Reset(x.pacemaker.GetRoundTimeout())
	round := x.pacemaker.GetCurrentRound()
	if x.leaderSelector.GetLeaderForRound(round) != x.peer.ID() {
		logger.Info("%v round %v new round start, not leader awaiting proposal", x.peer.String(), round)
		return
	}
	logger.Info("%v round %v new round start, node is leader", x.peer.String(), x.pacemaker.GetCurrentRound())
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
		logger.Warning("%v failed to send proposal message, message signing failed: %v", x.peer.String(), err)
	}
	// broadcast proposal message (also to self)
	receivers := make([]peer.ID, len(x.leaderSelector.GetRootNodes()))
	for i, validator := range x.leaderSelector.GetRootNodes() {
		id, _ := validator.GetID()
		receivers[i] = id
	}
	logger.Trace("%v broadcasting proposal msg", x.peer.String())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg}, receivers); err != nil {
		logger.Warning("%v failed to send proposal message, network error: %v", x.peer.String(), err)
	}
}

func (x *ConsensusManager) onCertificateReq(req *atomic_broadcast.GetCertificates) {
	logger.Trace("%v round %v certificate request from %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), req.NodeId)
	certs := x.blockStore.GetCertificates()
	payload := make([]*certificates.UnicityCertificate, 0, len(certs))
	for _, c := range certs {
		payload = append(payload, c)
	}
	respMsg := &atomic_broadcast.CertificatesMsg{Certificates: payload}
	peerID, err := peer.Decode(req.NodeId)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", req.NodeId)
		return
	}
	if err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootCertResp,
			Message:  respMsg}, []peer.ID{peerID}); err != nil {
		logger.Warning("%v failed to send certificates response message, network error: %v", x.peer.String(), err)
	}
}

func (x *ConsensusManager) onCertificateResp(certs []*certificates.UnicityCertificate) {
	// check validity
	for _, c := range certs {
		if err := c.IsValid(x.trustBase.GetVerifiers(), x.params.HashAlgorithm, c.UnicityTreeCertificate.SystemIdentifier, c.UnicityTreeCertificate.SystemDescriptionHash); err != nil {
			logger.Warning("received invalid certificate, dropping all")
			return
		}
	}
	x.blockStore.UpdateCertificates(certs)
}

func (x *ConsensusManager) onStateReq(req *atomic_broadcast.GetStateMsg) {
	logger.Trace("%v round %v received state request from %v",
		x.peer.String(), x.pacemaker.GetCurrentRound(), req.NodeId)
	committedBlock := x.blockStore.GetRoot()
	pendingBlocks := x.blockStore.GetPendingBlocks()
	pending := make([]*atomic_broadcast.RecoveryBlock, len(pendingBlocks))
	for i, b := range pendingBlocks {
		pending[i] = &atomic_broadcast.RecoveryBlock{
			Block:    b.BlockData,
			Ir:       storage.ToRecoveryInputData(b.CurrentIR),
			Qc:       b.Qc,
			CommitQc: nil,
		}
	}
	respMsg := &atomic_broadcast.StateMsg{
		CommittedHead: &atomic_broadcast.RecoveryBlock{
			Block:    committedBlock.BlockData,
			Ir:       storage.ToRecoveryInputData(committedBlock.CurrentIR),
			Qc:       committedBlock.Qc,
			CommitQc: committedBlock.CommitQc,
		},
		BlockNode: pending,
	}
	peerID, err := peer.Decode(req.NodeId)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", req.NodeId)
		return
	}
	if err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootStateResp,
			Message:  respMsg}, []peer.ID{peerID}); err != nil {
		logger.Warning("%v failed to send state response message, network error: %v", x.peer.String(), err)
	}
}

func (x *ConsensusManager) onStateResponse(req *atomic_broadcast.StateMsg) {
	logger.Trace("%v round %v received state response",
		x.peer.String(), x.pacemaker.GetCurrentRound())
	if err := x.blockStore.RecoverState(req.CommittedHead, req.BlockNode, x.irReqVerifier); err != nil {
		logger.Warning("state response failed, %v", err)
	}
}
