package distributed

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"
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
		roundState    *RoundState
		proposer      Leader
		rootVerifier  *RootNodeVerifier
		irReqBuffer   *ProposalGenerator
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
	roundState := NewRoundState(lastState.LatestRound, conf.LocalTimeoutMs)
	timers := timer.NewTimers()
	timers.Start(localTimeoutId, roundState.GetRoundTimeout())
	// read state
	state, err := conf.stateStore.Get()
	if err != nil {
		return nil, err
	}
	ledger, err := NewRoundPipeline(conf.HashAlgorithm, lastState, partitionStore)
	if err != nil {
		return nil, err
	}
	// Am I the next leader?
	if l.GetLeaderForRound(state.LatestRound+1) == host.ID() {
		// Start timer and wait for requests from partition nodes
		timers.Start(blockRateId, conf.BlockRateMs)
	}
	consensusManager := &ConsensusManager{
		certReqCh:     make(chan consensus.IRChangeRequest, 1),
		certResultCh:  make(chan certificates.UnicityCertificate, 1),
		config:        conf,
		peer:          host,
		timers:        timers,
		net:           net,
		roundState:    roundState,
		proposer:      l,
		rootVerifier:  rootVerifier,
		irReqBuffer:   NewProposalGenerator(),
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
			receivers = append(receivers, x.proposer.GetLeaderForRound(x.roundState.GetCurrentRound()+1))
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
				x.timers.Restart(timerId)
			case timerId == blockRateId:
				logger.Warning("Throttling not yet implemented")
			default:
				logger.Warning("Unknown timer %v", timerId)
			}
		}
	}
}

// onLocalTimeout handle timeouts
func (x *ConsensusManager) onLocalTimeout() {
	// always restart timer
	// Has the validator voted in this round
	voteMsg := x.roundState.GetVoted()
	if voteMsg == nil {
		// todo: generate empty proposal and execute it (not yet specified)
		// remove below lines once able to generate a empty proposal and execute
		return
	}
	// here voteMsg is not nil
	// either validator has already voted in this round already or a dummy vote was created
	if !voteMsg.IsTimeout() {
		// add timeout with signature and resend vote
		timeout, err := atomic_broadcast.NewTimeout(voteMsg.VoteInfo.RootRound, voteMsg.VoteInfo.Epoch, x.roundPipeline.HighQC)
		if err != nil {
			logger.Warning("Local timeout error %v", err)
			return
		}
		sig, err := x.safety.SignTimeout(timeout, x.roundState.LastRoundTC())
		if err != nil {
			logger.Warning("Local timeout error %v", err)
			return
		}
		// Add timeout to vote
		if err := voteMsg.AddTimeoutSignature(timeout, sig); err != nil {
			logger.Warning("Atomic broadcast local timeout handing failed: %v", err)
			return
		}
	}
	// Record vote
	x.roundState.SetVoted(voteMsg)
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
	if x.proposer.IsValidLeader(x.peer.ID(), x.roundState.GetCurrentRound()+1) == false {
		logger.Warning("Validator is not leader in next round %v, IR change req ignored",
			x.roundState.GetCurrentRound()+1)
		return
	}
	// todo: AB-547 I am leader now, but have not yet proposed -> should still accept requests (throttling)
	// validate incoming request
	sysId := p.SystemIdentifier(irChange.SystemIdentifier)
	partitionInfo, err := x.partitions.GetPartitionInfo(sysId)
	if err != nil {
		logger.Warning("IR change error, failed to get total nods for partition %X, error: %v", sysId.Bytes(), err)
		return
	}
	if err != nil {
		return
	}
	if err := irChange.Verify(partitionInfo.TrustBase); err != nil {
		logger.Warning("Invalid IR change request error: %v", err)
		return
	}
	// Check partition change already in pipeline
	if x.roundPipeline.IsChangeInPipeline(sysId) {
		logger.Warning("Invalid IR change request partition %X: change in pipeline",
			sysId.Bytes())
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
	// Buffer and wait for opportunity to make the next proposal
	if err := x.irReqBuffer.ValidateAndBufferIRReq(irChange, luc, len(partitionInfo.TrustBase)); err != nil {
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
	x.checkRootState(vote.HighQc)
	// Was the proposal received? If not, should we recover it? or do we accept that this round will end up as timeout
	round := vote.VoteInfo.RootRound
	// Normal votes are only sent to the next leader,
	// timeout votes are broadcast to everybody
	if vote.IsTimeout() == false {
		nextRound := round + 1
		// verify that the validator is correct leader in next round
		if x.proposer.IsValidLeader(x.peer.ID(), nextRound) == false {
			logger.Warning("Received vote, validator is not leader in next round %v, vote ignored", nextRound)
			return
		}
	}
	// Store vote, check for QC and TC.
	qc, tc := x.roundState.RegisterVote(vote, x.rootVerifier)
	if qc != nil {
		logger.Warning("Round %v quorum achieved", vote.VoteInfo.RootRound)
		// advance round
		x.processCertificateQC(qc)
	}
	if tc != nil {
		logger.Warning("Round %v timeout quorum achieved", vote.VoteInfo.RootRound)
		x.processTC(tc)
	}
	// This node is the new leader in this round/view, generate proposal
	x.processNewRoundEvent()
}

func (x *ConsensusManager) checkRootState(qc *atomic_broadcast.QuorumCert) {
	execStateId := x.roundPipeline.GetExecStateId()
	if !bytes.Equal(qc.VoteInfo.ExecStateId, execStateId) {
		logger.Warning("QC round %v state is different, recover state", qc.VoteInfo.RootRound)
		logger.Error("Recovery not yet implemented")
		// todo: AB-320 try to recover
		return
	}
}

func (x *ConsensusManager) VerifyProposal(payload *atomic_broadcast.Payload) (map[p.SystemIdentifier]*certificates.InputRecord, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	committedState, err := x.config.stateStore.Get()
	if err != nil {
		return nil, err
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot
	// trust the proposer is honest
	requests := request_store.NewCertificationRequestStore()
	// Remember all partitions that have changes in the current proposal and apply changes
	changes := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for _, irChReq := range payload.Requests {
		systemId := p.SystemIdentifier(irChReq.SystemIdentifier)
		// verify certification Request
		// Find if the SystemIdentifier is known by partition store
		partitionInfo, err := x.partitions.GetPartitionInfo(systemId)
		if err != nil {
			return nil, fmt.Errorf("invalid payload: unknown partition %X", systemId.Bytes())
		}
		if err := irChReq.Verify(partitionInfo.TrustBase); err != nil {
			return nil, fmt.Errorf("invalid payload: partition %X certification request verifiaction failed %w", systemId.Bytes(), err)
		}

		for _, req := range irChReq.Requests {
			if systemId != p.SystemIdentifier(req.SystemIdentifier) {
				return nil, fmt.Errorf("invalid payload: ir change req. proof system id does not match %X", req.SystemIdentifier)
			}
			if err := requests.Add(req); err != nil {
				return nil, fmt.Errorf("invalid payload: partition %X, equivocating requets in proof", systemId.Bytes())
			}
		}
		inputRecord, consensusPossible := requests.IsConsensusReceived(systemId, len(partitionInfo.TrustBase))
		// match request type to proof
		switch irChReq.CertReason {
		case atomic_broadcast.IRChangeReqMsg_QUORUM:
			if inputRecord == nil {
				return nil, errors.Errorf("invalid payload: partition %X is missing proof for quorum", irChReq.SystemIdentifier)
			}
			break

		case atomic_broadcast.IRChangeReqMsg_QUORUM_NOT_POSSIBLE:
			if inputRecord != nil && consensusPossible == false {
				return nil, errors.Errorf("invalid payload: partition %X missing proof for no quorum", irChReq.SystemIdentifier)
			}
			break
		case atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT:
			// timout does not carry proof in form of certification requests
			// validate timeout against round number (or timestamp in unicity seal)
			cert, found := committedState.Certificates[systemId]
			if !found {
				return nil, errors.Errorf("invalid payload: partition %X state is missing", systemId)
			}
			// verify timeout ok
			lucAgeInRounds := x.roundState.GetCurrentRound() - cert.UnicitySeal.RootChainRoundNumber
			if lucAgeInRounds*500 < uint64(partitionInfo.SystemDescription.T2Timeout) {
				return nil, errors.Errorf("invalid payload: partition %X, time from latest UC %v, timeout %v, invalid timeout request",
					systemId, lucAgeInRounds*500, partitionInfo.SystemDescription.T2Timeout)
			}
			// copy last input record
			inputRecord = cert.InputRecord
			break
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
	// Check sync
	x.checkRootState(proposal.Block.Qc)
	// Every proposal must carry a QC for previous round
	// Process QC first, update round
	x.processCertificateQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	changes, err := x.VerifyProposal(proposal.Block.Payload)
	// execute proposal
	execStateId, err := x.roundPipeline.Add(x.roundState.GetCurrentRound(), changes)
	if err != nil {
		logger.Warning("Failed to execute proposal: %v", err.Error())
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, string(x.peer.ID()), x.roundState.LastRoundTC())
	if err != nil {
		logger.Warning("Failed to sign vote, vote not sent: %v", err.Error())
		return
	}
	// Add high Qc for state synchronization
	voteMsg.HighQc = x.roundPipeline.HighQC

	x.roundState.SetVoted(voteMsg)
	// send vote to the next leader
	receivers := make([]peer.ID, 1)
	receivers = append(receivers, x.proposer.GetLeaderForRound(x.roundState.GetCurrentRound()+1))
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
	committedState := x.roundPipeline.Update(qc)
	if committedState != nil {
		if err := x.config.stateStore.Save(*committedState); err != nil {
			logger.Warning("Failed persist new root chain state: %w", err)
		}
		for _, cert := range committedState.Certificates {
			x.certResultCh <- *cert
		}
	}
	x.roundState.AdvanceRoundQC(qc)
}

func (x *ConsensusManager) processTC(tc *atomic_broadcast.TimeoutCert) {
	if tc != nil {
		// restore state pipeline to last persisted state
		lastState, err := x.config.stateStore.Get()
		if err != nil {
			// todo: keep last persisted state at hand all the time?
			return
		}
		x.roundPipeline.Reset(lastState)
		x.roundState.AdvanceRoundTC(tc)
	}
}

func (x *ConsensusManager) processNewRoundEvent() {
	round := x.roundState.GetCurrentRound()
	if x.proposer.IsValidLeader(x.peer.ID(), round) {
		// block id is set when signing, epoch is currently set to 0
		block := &atomic_broadcast.BlockData{
			Author:    string(x.peer.ID()),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(),
			Qc:        x.roundPipeline.HighQC,
		}
		hash, err := block.Hash(x.config.HashAlgorithm)
		if err != nil {
			logger.Warning("Failed to send proposal message, block id error: %v", err)
			return
		}
		block.Id = hash
		proposalMsg := &atomic_broadcast.ProposalMsg{
			Block:       block,
			LastRoundTc: x.roundState.LastRoundTC(),
		}
		if err := x.safety.SignProposal(proposalMsg); err != nil {
			logger.Warning("Failed to send proposal message, message signing failed: %v", err)
		}
		// Broadcast message
		allValidators := x.proposer.GetRootNodes()
		receivers := make([]peer.ID, len(allValidators))
		receivers = append(receivers, allValidators...)
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
}
