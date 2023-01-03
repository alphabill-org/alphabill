package distributed

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"

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
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// local timeout
	blockRateId         = "block-rate"
	localTimeoutId      = "local-timeout"
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

	PartitionStore interface {
		GetPartitionInfo(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	AbConsensusConfig struct {
		BlockRateMs        time.Duration
		LocalTimeoutMs     time.Duration
		ConsensusThreshold uint32
		RootTrustBase      map[string]crypto.Verifier
		HashAlgorithm      gocrypto.Hash
		stateStore         StateStore
	}

	Option func(c *AbConsensusConfig)

	ConsensusManager struct {
		ctx           context.Context
		ctxCancel     context.CancelFunc
		certReqCh     chan consensus.IRChangeRequest
		certResultCh  chan certificates.UnicityCertificate
		config        *AbConsensusConfig
		peer          *network.Peer
		timers        *timer.Timers
		net           RootNet
		pacemaker     *Pacemaker
		proposer      Leader
		rootVerifier  *RootNodeVerifier
		irReqBuffer   *IrReqBuffer
		safety        *SafetyModule
		roundPipeline *RoundPipeline
		partitions    PartitionStore
	}
)

func WithStateStore(store StateStore) Option {
	return func(c *AbConsensusConfig) {
		c.stateStore = store
	}
}

func loadConf(genesisRoot *genesis.GenesisRootRecord, opts []Option) *AbConsensusConfig {
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
	hashAlgo := gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm)
	conf := &AbConsensusConfig{
		BlockRateMs:        time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeoutMs:     localTimeout,
		ConsensusThreshold: quorumThreshold,
		RootTrustBase:      rootTrustBase,
		HashAlgorithm:      hashAlgo,
		stateStore:         store.NewInMemStateStore(hashAlgo),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(host *network.Peer, genesisRoot *genesis.GenesisRootRecord,
	partitionStore PartitionStore, signer crypto.Signer, net RootNet, opts ...Option) (*ConsensusManager, error) {
	// Sanity checks
	if genesisRoot == nil {
		return nil, errors.New("cannot start distributed consensus, genesis root record is nil")
	}
	// load configuration
	conf := loadConf(genesisRoot, opts)
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
	safetyModule, err := NewSafetyModule(signer)
	if err != nil {
		return nil, err
	}

	lastState, err := conf.stateStore.Get()
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
	pace := NewPacemaker(lastState.LatestRound, conf.LocalTimeoutMs, conf.BlockRateMs)
	timers := timer.NewTimers()
	ledger, err := NewRoundPipeline(conf.HashAlgorithm, lastState, partitionStore)
	if err != nil {
		return nil, err
	}

	consensusManager := &ConsensusManager{
		certReqCh:     make(chan consensus.IRChangeRequest, 1),
		certResultCh:  make(chan certificates.UnicityCertificate, 1),
		config:        conf,
		peer:          host,
		timers:        timers,
		net:           net,
		pacemaker:     pace,
		proposer:      l,
		rootVerifier:  rootVerifier,
		irReqBuffer:   NewIrReqBuffer(),
		safety:        safetyModule,
		roundPipeline: ledger,
		partitions:    partitionStore,
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) Start() {
	// Start timers and network processing
	x.timers.Start(localTimeoutId, x.config.LocalTimeoutMs)
	// Am I the next leader?
	currentRound := x.pacemaker.GetCurrentRound()
	if x.proposer.GetLeaderForRound(currentRound) == x.peer.ID() {
		x.timers.Start(blockRateId, x.pacemaker.CalcTimeTilNextProposal())
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
				return
			}
			switch msg.Protocol {
			case network.ProtocolRootIrChangeReq:
				req, correctType := msg.Message.(*atomic_broadcast.IRChangeReqMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling IR Change Request"), req)
				logger.Debug("IR change request from %v", msg.From)
				x.onIRChange(req)
				break
			case network.ProtocolRootProposal:
				req, correctType := msg.Message.(*atomic_broadcast.ProposalMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling Proposal from %s", req.Block.Author), req)
				logger.Debug("Proposal received from %v", msg.From)
				x.onProposalMsg(req)
				break
			case network.ProtocolRootVote:
				req, correctType := msg.Message.(*atomic_broadcast.VoteMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Received Vote from %s", req.Author), req)
				logger.Debug("Vote received from %v", msg.From)
				x.onVoteMsg(req)
				break
			case network.ProtocolRootTimeout:
				req, correctType := msg.Message.(*atomic_broadcast.TimeoutMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Received Timeout vote from %s", req.Author), req)
				logger.Debug("Vote received from %v", msg.From)
				x.onTimeoutMsg(req)
				break
			case network.ProtocolRootStateReq:
				req, correctType := msg.Message.(*atomic_broadcast.StateRequestMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Received recovery request"), req)
				logger.Debug("State request from", msg.From)
				// Send state, with proof (signed by other validators)
				// Todo: AB-320 add handling
				break
			case network.ProtocolRootStateResp:
				req, correctType := msg.Message.(*atomic_broadcast.StateReplyMsg)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Received recovery response"), req)
				logger.Debug("State reply from %v", msg.From)
				// Verify and store
				// Todo: AB-320 Add handling
				break
			default:
				logger.Warning("Unknown protocol req %s from %v", msg.Protocol, msg.From)
				break
			}
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("certification channel closed, exiting distributed consensus main loop %v")
				return
			}
			logger.Debug("IR change request from partition")
			// must be forwarded to the next round leader
			receivers := make([]peer.ID, 1)
			receivers = append(receivers, x.proposer.GetLeaderForRound(x.pacemaker.GetCurrentRound()+1))
			// assume positive case as this will be most common
			reason := atomic_broadcast.IRChangeReqMsg_QUORUM
			if req.Reason == consensus.QuorumNotPossible {
				reason = atomic_broadcast.IRChangeReqMsg_QUORUM_NOT_POSSIBLE
			}
			irReq := &atomic_broadcast.IRChangeReqMsg{
				SystemIdentifier: req.SystemIdentifier.Bytes(),
				CertReason:       reason,
				Requests:         req.Requests}

			err := x.net.Send(
				network.OutputMessage{
					Protocol: network.ProtocolRootVote,
					Message:  irReq,
				}, receivers)
			if err != nil {
				logger.Warning("Failed to forward IR Change request: %v", err)
			}
		// handle timeouts
		case nt := <-x.timers.C:
			if nt == nil {
				continue
			}
			timerId := nt.Name()
			switch {
			case timerId == localTimeoutId:
				logger.Warning("handle local timeout")
				x.onLocalTimeout()
			case timerId == blockRateId:
				// throttling, make a proposal
				x.processNewRoundEvent()
			default:
				logger.Warning("Unknown timer %v", timerId)
			}
		}
	}
}

// onLocalTimeout handle timeouts
func (x *ConsensusManager) onLocalTimeout() {
	// always restart timer, time might be adjusted in case
	defer x.timers.Restart(localTimeoutId)

	// Has the validator voted in this round, if true send the same vote
	// maybe less than quorum of nodes where operational the last time
	voteMsg := x.pacemaker.GetTimeoutVote()
	if voteMsg == nil {
		// create timeout vote
		voteMsg = atomic_broadcast.NewTimeoutMsg(atomic_broadcast.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.roundPipeline.HighQC), string(x.peer.ID()))
		// sign
		if err := x.safety.SignTimeout(voteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("Local timeout error %v", err)
			return
		}
		// Record vote
		x.pacemaker.SetTimeoutVote(voteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	allValidators := x.proposer.GetRootNodes()
	receivers := make([]peer.ID, len(allValidators))
	receivers = append(receivers, allValidators...)
	logger.Info("Broadcasting timeout vote")
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
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
	if x.proposer.IsValidLeader(x.peer.ID(), x.pacemaker.GetCurrentRound()+1) == false {
		logger.Warning("Validator is not leader in next round %v, IR change req ignored",
			x.pacemaker.GetCurrentRound()+1)
		return
	}
	// validate incoming request
	sysId := p.SystemIdentifier(irChange.SystemIdentifier)
	partitionInfo, err := x.partitions.GetPartitionInfo(sysId)
	if err != nil {
		logger.Warning("IR change error, failed to get total nods for partition %X, error: %v", sysId.Bytes(), err)
		return
	}
	state, err := x.config.stateStore.Get()
	if err != nil {
		logger.Warning("IR change request ignored, failed to read current state: %w", err)
		return
	}
	luc, f := state.Certificates[sysId]
	if !f {
		logger.Warning("IR change request ignored, no last state for system id %X", sysId.Bytes())
		return
	}
	// calculate LUC age using rounds and min block rate
	lucAgeInRounds := time.Duration(x.pacemaker.GetCurrentRound()-luc.UnicitySeal.RootChainRoundNumber) * x.config.BlockRateMs
	inputRecord, err := irChange.Verify(partitionInfo, luc, lucAgeInRounds)
	if err != nil {
		logger.Warning("Invalid IR change request error: %v", err)
		return
	}
	// Check partition change already in pipeline
	if x.roundPipeline.IsChangeInPipeline(sysId) {
		logger.Warning("Invalid IR change request partition %X: change in pipeline",
			sysId.Bytes())
		return
	}
	// Buffer and wait for opportunity to make the next proposal
	if err := x.irReqBuffer.Add(IRChange{InputRecord: inputRecord, Reason: irChange.CertReason, Msg: irChange}); err != nil {
		logger.Warning("IR change request from partition %X error: %w", sysId.Bytes(), err)
		return
	}
}

// onVoteMsg handle votes and timeout votes
func (x *ConsensusManager) onVoteMsg(vote *atomic_broadcast.VoteMsg) {
	// verify signature on vote
	err := vote.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Atomic broadcast Vote verify failed: %v", err)
	}
	// SyncState
	if x.checkRecoveryNeeded(vote.HighQc) {
		logger.Error("Recovery not yet implemented")
		// todo: AB-320 try to recover
		return
	}
	// Was the proposal received? If not, should we recover it? or do we accept that this round will end up as timeout
	round := vote.VoteInfo.RootRound
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	nextRound := round + 1
	// verify that the validator is correct leader in next round
	if x.proposer.IsValidLeader(x.peer.ID(), nextRound) == false {
		// this might also be a stale vote, since when we have quorum the round is advanced and the node becomes
		// the new leader in the current view/round
		logger.Warning("Received vote, validator is not leader in next round %v, vote ignored", nextRound)
		return
	}
	// Store vote, check for QC
	qc := x.pacemaker.RegisterVote(vote, x.rootVerifier)
	if qc != nil {
		logger.Info("Round %v quorum achieved", vote.VoteInfo.RootRound)
		// advance round (only done once)
		// NB! it must be able to
		x.processCertificateQC(qc)
		// NB! time returned might be 0 in which case there will be small delay added
		// this can be optimized later
		x.timers.Start(blockRateId, x.pacemaker.CalcTimeTilNextProposal())
	}
}

func (x *ConsensusManager) onTimeoutMsg(vote *atomic_broadcast.TimeoutMsg) {
	// verify signature on vote
	err := vote.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Atomic broadcast Vote verify failed: %v", err)
	}
	// Author voted timeout, proceed
	logger.Info("Received timout vote from %v, round %v",
		vote.Author, vote.Timeout.Round)
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
	execStateId := x.roundPipeline.GetExecStateId()
	if !bytes.Equal(qc.VoteInfo.ExecStateId, execStateId) {
		logger.Warning("QC round %v state is different, recover state", qc.VoteInfo.RootRound)
		return true
	}
	return false
}

func (x *ConsensusManager) VerifyProposalPayload(payload *atomic_broadcast.Payload) (map[p.SystemIdentifier]*certificates.InputRecord, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	committedState, err := x.config.stateStore.Get()
	if err != nil {
		return nil, err
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the proposer is honest
	// Remember all partitions that have changes in the current proposal and apply changes
	changes := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for _, irChReq := range payload.Requests {
		systemId := p.SystemIdentifier(irChReq.SystemIdentifier)
		// verify certification Request
		luc, found := committedState.Certificates[systemId]
		if !found {
			return nil, errors.Errorf("invalid payload: partition %X state is missing", systemId)
		}
		// Find if the SystemIdentifier is known by partition store
		partitionInfo, err := x.partitions.GetPartitionInfo(systemId)
		if err != nil {
			return nil, fmt.Errorf("invalid payload: unknown partition %X", systemId.Bytes())
		}
		lucAgeInRounds := time.Duration(x.pacemaker.GetCurrentRound()-luc.UnicitySeal.RootChainRoundNumber) * x.config.BlockRateMs
		// verify request
		inputRecord, err := irChReq.Verify(partitionInfo, luc, lucAgeInRounds)
		if err != nil {
			return nil, fmt.Errorf("invalid payload: partition %X certification request verifiaction failed %w", systemId.Bytes(), err)
		}
		changes[systemId] = inputRecord
	}
	return changes, nil
}

func (x *ConsensusManager) onProposalMsg(proposal *atomic_broadcast.ProposalMsg) {
	// verify signature on proposal (does not verify partition request signatures)
	err := proposal.Verify(x.rootVerifier.GetQuorumThreshold(), x.rootVerifier.GetVerifiers())
	if err != nil {
		logger.Warning("Invalid Proposal message, verify failed: %v", err)
	}
	util.WriteDebugJsonLog(logger, fmt.Sprintf("Received Proposal from %s", proposal.Block.Author), proposal)
	logger.Info("Received proposal message from %v", proposal.Block.Author)
	// Is from valid proposer
	if x.proposer.IsValidLeader(peer.ID(proposal.Block.Author), proposal.Block.Round) == false {
		logger.Warning("Proposal author %V is not a valid proposer for round %v, ignoring proposal",
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
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, string(x.peer.ID()), x.pacemaker.LastRoundTC())
	if err != nil {
		logger.Warning("Failed to sign vote, vote not sent: %v", err.Error())
		return
	}
	// Add high Qc for state synchronization
	voteMsg.HighQc = x.roundPipeline.HighQC

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	receivers := make([]peer.ID, 1)
	receivers = append(receivers, x.proposer.GetLeaderForRound(x.pacemaker.GetCurrentRound()+1))
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, receivers)
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
		if err := x.config.stateStore.Save(*committedState); err != nil {
			logger.Warning("Failed persist new root chain state: %w", err)
		}
		for _, cert := range committedState.Certificates {
			x.certResultCh <- *cert
		}
	}
	x.pacemaker.AdvanceRoundQC(qc)
	// progress is made, restart timeout (move to pacemaker)
	x.timers.Restart(localTimeoutId)
}

func (x *ConsensusManager) processTC(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	// restore state pipeline to last persisted state
	lastState, err := x.config.stateStore.Get()
	if err != nil {
		// todo: keep last persisted state at hand all the time?
		return
	}
	x.roundPipeline.Reset(lastState)
	x.pacemaker.AdvanceRoundTC(tc)
	// progress is made, restart timeout (move to pacemaker)
	x.timers.Restart(localTimeoutId)
}

func (x *ConsensusManager) processNewRoundEvent() {
	round := x.pacemaker.GetCurrentRound()
	if !x.proposer.IsValidLeader(x.peer.ID(), round) {
		logger.Warning("Current node is not the leader in round %v, skip proposal", round)
		return
	}
	// to counteract throttling (find a better solution)
	x.timers.Restart(localTimeoutId)
	// create proposal message, generate payload of IR change requests
	proposalMsg := &atomic_broadcast.ProposalMsg{
		Block: &atomic_broadcast.BlockData{
			Author:    string(x.peer.ID()),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(),
			Qc:        x.roundPipeline.HighQC,
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err := x.safety.SignProposal(proposalMsg); err != nil {
		logger.Warning("Failed to send proposal message, message signing failed: %v", err)
	}
	// broadcast proposal message (also to self)
	allValidators := x.proposer.GetRootNodes()
	receivers := make([]peer.ID, len(allValidators))
	receivers = append(receivers, allValidators...)
	logger.Info("Broadcasting proposal msg")
	err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg,
		}, receivers)
	if err != nil {
		logger.Warning("Failed to send proposal message, network error: %v", err)
	}
}
