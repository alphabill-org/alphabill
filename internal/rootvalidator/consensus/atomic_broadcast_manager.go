package consensus

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/leader"

	"github.com/alphabill-org/alphabill/internal/rootvalidator"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"

	"github.com/alphabill-org/alphabill/internal/certificates"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"

	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// local timeout
	blockRateId         = "block-rate"
	localTimeoutId      = "local-timeout"
	defaultRoundTimeout = 500 * time.Millisecond
	defaultLocalTimeout = 10000 * time.Millisecond
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

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	ConsensusConf struct {
		BlockRateMs        time.Duration
		LocalTimeoutMs     time.Duration
		ConsensusThreshold uint32
		RootTrustBase      map[string]crypto.Verifier
		HashAlgorithm      gocrypto.Hash
	}

	ConsensusOption func(c *ConsensusConf)

	AtomicBroadcastManager struct {
		ctx          context.Context
		ctxCancel    context.CancelFunc
		certReqCh    chan rootvalidator.CertificationRequest
		certResultCh chan certificates.UnicityCertificate
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

func loadConsensusConf(genesisRoot *genesis.GenesisRootRecord, opts []ConsensusOption) *ConsensusConf {
	rootTrustBase, err := genesis.NewValidatorTrustBase(genesisRoot.RootValidators)
	if err != nil {
		return nil
	}
	nodesMap := make(map[peer.ID][]byte)
	for _, n := range genesisRoot.RootValidators {
		nodesMap[peer.ID(n.NodeIdentifier)] = n.SigningPublicKey
	}
	localTimeout := defaultLocalTimeout
	quorumThreshold := genesis.GetMinQuorumThreshold(uint32(len(nodesMap)))
	// Is consensus timeout specified?
	if genesisRoot.Consensus.ConsensusTimeoutMs != nil {
		localTimeout = time.Duration(*genesisRoot.Consensus.ConsensusTimeoutMs) * time.Millisecond
	}
	// Is consensus threshold specified
	if genesisRoot.Consensus.QuorumThreshold != nil {
		quorumThreshold = *genesisRoot.Consensus.QuorumThreshold
	}
	conf := &ConsensusConf{
		BlockRateMs:        time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeoutMs:     localTimeout,
		ConsensusThreshold: quorumThreshold,
		RootTrustBase:      rootTrustBase,
		HashAlgorithm:      gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}

func NewAtomicBroadcastManager(host *network.Peer, genesisRoot *genesis.GenesisRootRecord, stateStore StateStore,
	partitionStore *rootvalidator.PartitionStore, signer crypto.Signer, net RootNet, opts ...ConsensusOption) (*AtomicBroadcastManager, error) {
	// Sanity checks
	if genesisRoot == nil {
		return nil, errors.New("cannot start distributed consensus, genesis root record is nil")
	}
	// load configuration
	conf := loadConsensusConf(genesisRoot, opts)
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
	consensusManager := &AtomicBroadcastManager{
		certReqCh:    make(chan rootvalidator.CertificationRequest, 1),
		certResultCh: make(chan certificates.UnicityCertificate, 4),
		peer:         host,
		store:        stateStore,
		timers:       timers,
		net:          net,
		roundState:   roundState,
		proposer:     l,
		rootVerifier: rootVerifier,
		safety:       NewSafetyModule(signer),
		stateLedger:  ledger,
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	return consensusManager, nil
}

func (a *AtomicBroadcastManager) RequestCertification() chan<- rootvalidator.CertificationRequest {
	return a.certReqCh
}

func (a *AtomicBroadcastManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return a.certResultCh
}

func (a *AtomicBroadcastManager) Start() {
	go a.loop()
}

func (a *AtomicBroadcastManager) Stop() {
	a.timers.WaitClose()
	a.ctxCancel()
}

func (a *AtomicBroadcastManager) loop() {
	for {
		select {
		case <-a.ctx.Done():
			logger.Info("Exiting distributed consensus manager main loop")
			return
		case msg, ok := <-a.net.ReceivedChannel():
			if !ok {
				logger.Warning("Root network received channel closed, exiting distributed consensus main loop")
				return
			}
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
				a.onIRChange(req)
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
		case req, ok := <-a.certReqCh:
			if !ok {
				logger.Warning("certification channel closed, exiting distributed consensus main loop %v", req)
				return
			}

		// handle timeouts
		case nt := <-a.timers.C:
			if nt == nil {
				continue
			}
			timerId := nt.Name()
			switch {
			case timerId == localTimeoutId:
				logger.Warning("handle local timeout")
				a.onLocalTimeout()
				a.timers.Restart(timerId)
			case timerId == blockRateId:
				logger.Warning("Throttling not yet implemented")
			default:
				logger.Warning("Unknown timer %v", timerId)
			}
		}
	}
}

// onLocalTimeout handle timeouts
func (a *AtomicBroadcastManager) onLocalTimeout() {
	// always restart timer
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
	logger.Info("Broadcasting timeout vote")
	err := a.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send vote message: %v", err)
	}
}

// onIRChange handles IR change request from other root nodes
func (a *AtomicBroadcastManager) onIRChange(irChange *atomic_broadcast.IRChangeReqMsg) {
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
