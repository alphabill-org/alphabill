package abdrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/exp/slices"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/leader"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	recoveryInfo struct {
		toRound    uint64
		triggerMsg any
	}

	RootNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		Broadcast(msg network.OutputMessage) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	// Leader provides interface to different leader selection algorithms
	Leader interface {
		// GetLeaderForRound returns valid leader (node id) for round/view number
		GetLeaderForRound(round uint64) peer.ID
		// currentRound - what PaceMaker considers to be the current round at the time QC is processed.
		Update(qc *abtypes.QuorumCert, currentRound uint64) error
	}

	ConsensusManager struct {
		certReqCh      chan consensus.IRChangeRequest
		certResultCh   chan *types.UnicityCertificate
		params         *consensus.Parameters
		id             peer.ID
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		trustBase      *RootTrustBase
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		blockStore     *storage.BlockStore
		partitions     partitions.PartitionConfiguration
		irReqVerifier  *IRChangeReqVerifier
		t2Timeouts     *PartitionTimeoutGenerator
		// votes need to be buffered when CM will be the next leader (so other nodes
		// will send votes to it) but it hasn't got the proposal yet so it can't process
		// the votes. voteBuffer maps author id to vote so we do not buffer same vote
		// multiple times (votes are buffered for single round only)
		voteBuffer map[string]*abdrc.VoteMsg
		// when set (ie not nil) CM is in recovery mode, trying to get into the same
		// state as other CMs
		recovery *recoveryInfo
	}
)

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(nodeID peer.ID, rg *genesis.RootGenesis,
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
	bStore, err := storage.NewBlockStore(cParams.HashAlgorithm, rg.Partitions, optional.Storage)
	if err != nil {
		return nil, fmt.Errorf("consensus block storage init failed: %w", err)
	}
	reqVerifier, err := NewIRChangeReqVerifier(cParams, partitionStore, bStore)
	if err != nil {
		return nil, fmt.Errorf("block verifier construct error: %w", err)
	}
	t2TimeoutGen, err := NewLucBasedT2TimeoutGenerator(cParams, partitionStore, bStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create T2 timeout generator: %w", err)
	}
	safetyModule, err := NewSafetyModule(nodeID.String(), signer, optional.Storage)
	if err != nil {
		return nil, err
	}

	leader, err := leaderSelector(rg, bStore.Block)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus leader selector: %w", err)
	}

	tb, err := NewRootTrustBaseFromGenesis(rg.Root)
	if err != nil {
		return nil, fmt.Errorf("consensus root trust base init failed, %w", err)
	}
	consensusManager := &ConsensusManager{
		certReqCh:      make(chan consensus.IRChangeRequest),
		certResultCh:   make(chan *types.UnicityCertificate),
		params:         cParams,
		id:             nodeID,
		net:            net,
		pacemaker:      NewPacemaker(cParams.BlockRate/2, cParams.LocalTimeout),
		leaderSelector: leader,
		trustBase:      tb,
		irReqBuffer:    NewIrReqBuffer(),
		safety:         safetyModule,
		blockStore:     bStore,
		partitions:     partitionStore,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
		voteBuffer:     make(map[string]*abdrc.VoteMsg),
	}
	return consensusManager, nil
}

func leaderSelector(rg *genesis.RootGenesis, blockLoader leader.BlockLoader) (ls Leader, err error) {
	// NB! both leader selector algorithms make the assumption that the rootNodes slice is
	// sorted and it's content doesn't change!
	rootNodes, err := rg.NodeIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get root node IDs: %w", err)
	}
	slices.Sort(rootNodes)

	switch len(rootNodes) {
	case 0:
		return nil, errors.New("number of peers must be greater than zero")
	case 1:
		// really should have "constant leader" algorithm but this shouldn't happen IRL.
		// we need this case as some tests create only one root node. Could use reputation
		// based selection with excludeSize=0 but round-robin is more efficient...
		return leader.NewRoundRobin(rootNodes, 1)
	default:
		// we're limited to window size and exclude size 1 as our block loader (block store) doesn't
		// keep history, ie we can't load blocks older than previous block.
		return leader.NewReputationBased(rootNodes, 1, 1, blockLoader)
	}
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan *types.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) GetLatestUnicityCertificate(id types.SystemID) (*types.UnicityCertificate, error) {
	ucs := x.blockStore.GetCertificates()
	luc, f := ucs[p.SystemIdentifier(id)]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

func (x *ConsensusManager) Run(ctx context.Context) error {
	defer x.pacemaker.Stop()
	x.pacemaker.Reset(x.blockStore.GetHighQc().VoteInfo.RoundNumber)
	currentRound := x.pacemaker.GetCurrentRound()
	logger.Info("%s round %d root node starting, leader is %s", x.id.ShortString(), currentRound, x.leaderSelector.GetLeaderForRound(currentRound))
	err := x.loop(ctx)
	logger.Info("%v exited distributed consensus manager main loop: %v", x.id.ShortString(), err)
	return err
}

func (x *ConsensusManager) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-x.net.ReceivedChannel():
			if !ok {
				return fmt.Errorf("root network received channel has been closed")
			}
			logMsg := fmt.Sprintf("%s round %d received %T from %v", x.id.ShortString(), x.pacemaker.GetCurrentRound(), msg.Message, msg.From)
			util.WriteTraceJsonLog(logger, logMsg, msg.Message)
			if err := x.handleRootNetMsg(ctx, msg.Message); err != nil {
				logger.Warning(logMsg+": %v", err)
			}
		case req := <-x.certReqCh:
			if err := x.onPartitionIRChangeReq(&req); err != nil {
				logger.Warning("%v round %d failed to process IR change request from partition: %v", x.id.ShortString(), x.pacemaker.GetCurrentRound(), err)
			}
		case event := <-x.pacemaker.StatusEvents():
			switch event {
			case pmsRoundMatured:
				currentRound := x.pacemaker.GetCurrentRound()
				leader := x.leaderSelector.GetLeaderForRound(currentRound + 1)
				logger.Debug("%v round %d has lasted minimum required duration; next leader %s", x.id.ShortString(), currentRound, leader.ShortString())
				// round 2 is system bootstrap and is a special case - as there is no proposal no one is sending votes
				// and thus leader won't achieve quorum and doesn't make next proposal (and the round would time out).
				// So we just have the round 2 leader to trigger next round when it's mature (root genesis QC will be
				// used as HighQc in the proposal).
				if leader == x.id || (currentRound == 2 && x.id == x.leaderSelector.GetLeaderForRound(2)) {
					if qc := x.pacemaker.RoundQC(); qc != nil || currentRound == 2 {
						x.processQC(qc)
						x.processNewRoundEvent()
					}
				}
			case pmsRoundTimeout:
				x.onLocalTimeout()
			}
		}
	}
}

/*
handleRootNetMsg routes messages from "root net" iow messages sent by other rootchain
validators to appropriate message handler.
*/
func (x *ConsensusManager) handleRootNetMsg(ctx context.Context, msg any) error {
	switch mt := msg.(type) {
	case *abtypes.IRChangeReq:
		return x.onIRChange(mt)
	case *abdrc.ProposalMsg:
		return x.onProposalMsg(ctx, mt)
	case *abdrc.VoteMsg:
		return x.onVoteMsg(mt)
	case *abdrc.TimeoutMsg:
		return x.onTimeoutMsg(mt)
	case *abdrc.GetStateMsg:
		return x.onStateReq(mt)
	case *abdrc.StateMsg:
		return x.onStateResponse(ctx, mt)
	}
	return fmt.Errorf("unknown message type %T", msg)
}

// onLocalTimeout handle local timeout, either no proposal is received or voting does not
// reach consensus. Triggers timeout voting.
func (x *ConsensusManager) onLocalTimeout() {
	logger.Info("%v round %v local timeout", x.id.ShortString(), x.pacemaker.GetCurrentRound())
	// has the validator voted in this round, if true send the same (timeout)vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = abdrc.NewTimeoutMsg(abtypes.NewTimeout(
			x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc()), x.id.String())
		// sign
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			logger.Warning("%v local timeout error, %v", x.id.ShortString(), err)
			return
		}
		x.pacemaker.SetTimeoutVote(timeoutVoteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	logger.Trace("%v round %v broadcasting timeout vote", x.id.ShortString(), x.pacemaker.GetCurrentRound())
	if err := x.net.Broadcast(
		network.OutputMessage{
			Protocol: network.ProtocolRootTimeout,
			Message:  timeoutVoteMsg,
		}); err != nil {
		logger.Warning("%v error on broadcasting timeout vote: %v", x.id.ShortString(), err)
	}
}

// onPartitionIRChangeReq handle partition change requests. Received from go routine handling
// partition communication when either partition reaches consensus or cannot reach consensus.
func (x *ConsensusManager) onPartitionIRChangeReq(req *consensus.IRChangeRequest) error {
	logger.Debug("%v round %v, IR change request from partition", x.id.String(), x.pacemaker.GetCurrentRound())
	irReq := &abtypes.IRChangeReq{
		SystemIdentifier: req.SystemIdentifier,
		Requests:         req.Requests,
	}
	switch req.Reason {
	case consensus.Quorum:
		irReq.CertReason = abtypes.Quorum
	case consensus.QuorumNotPossible:
		irReq.CertReason = abtypes.QuorumNotPossible
	default:
		return fmt.Errorf("unexpected IR change request reason: %v", req.Reason)
	}

	return x.onIRChange(irReq)
}

// onIRChange handles IR change request
func (x *ConsensusManager) onIRChange(irChangeMsg *abtypes.IRChangeReq) error {
	if err := irChangeMsg.IsValid(); err != nil {
		return fmt.Errorf("invalid IR change request: %w", err)
	}
	currentLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// todo: if in recovery then forward to next?
	// if the node is leader or will be the next leader then buffer the request to be included in the block proposal
	if currentLeader == x.id || nextLeader == x.id {
		logger.Debug("%v round %v IR change request received", x.id.ShortString(), x.pacemaker.GetCurrentRound())
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChangeMsg, x.irReqVerifier); err != nil {
			return fmt.Errorf("failed to add IR change request into buffer: %w", err)
		}
		return nil
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this is a completely lost message or because of race we just proposed, try to forward again to the next leader
	logger.Warning("%v is not leader in next round %v, IR change req forwarded again %v",
		x.id.ShortString(), x.pacemaker.GetCurrentRound()+1, nextLeader.String())
	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootIrChangeReq,
			Message:  irChangeMsg,
		}, []peer.ID{nextLeader}); err != nil {
		return fmt.Errorf("failed to forward IR change message to the next leader: %w", err)
	}
	return nil
}

// onVoteMsg handle votes messages from other root validators
func (x *ConsensusManager) onVoteMsg(vote *abdrc.VoteMsg) error {
	if vote.VoteInfo.RoundNumber < x.pacemaker.GetCurrentRound() {
		return fmt.Errorf("stale vote for round %d from %s", vote.VoteInfo.RoundNumber, vote.Author)
	}
	// verify signature on vote
	err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers())
	if err != nil {
		return fmt.Errorf("invalid vote: %w", err)
	}
	// if a vote is received for the next round it is intended for the node which is going to be the
	// leader in round current+2. We cache it in the voteBuffer anyway, in hope that this node will
	// be the leader then (reputation based algorithm can't predict leader for the round current+2
	// as ie round-robin can).
	if vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound()+1 {
		// either vote arrived before proposal or we're behind (others think we're the leader of
		// the round but we haven't seen QC or TC for previous round?)
		// Votes are buffered for one round only so if we overwrite author's vote it is either stale
		// or we have received the vote more than once.
		logger.Debug("%v round %v received vote for round %v before proposal, buffering vote",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber)
		x.voteBuffer[vote.Author] = vote
		// if we have received quorum votes, but no proposal yet, then try and recover the proposal.
		// NB! it seems that it's quite common that votes arrive before proposal and going into recovery
		// too early is counterproductive... maybe do not trigger recovery here at all - if we're lucky
		// proposal will arrive on time, otherwise round will likely TO anyway?
		if uint32(len(x.voteBuffer)) >= x.trustBase.GetQuorumThreshold() {
			err := fmt.Errorf("have received %d votes but no proposal, entering recovery", len(x.voteBuffer))
			if e := x.sendRecoveryRequests(vote); e != nil {
				err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
			}
			return err
		}
		return nil
	}

	// Normal votes are only sent to the next leader (timeout votes are broadcast) is it us?
	// NB! we assume vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound() but it also could be that VVR > CR+1
	nextRound := vote.VoteInfo.RoundNumber + 1
	if x.leaderSelector.GetLeaderForRound(nextRound) != x.id {
		return fmt.Errorf("validator is not the leader for round %d", nextRound)
	}

	// SyncState, compare last handled QC
	// this check should be before checking are we the leader for the VVR+1 round as when
	// we're out of sync the test for the next leader doesn't make sense? add check for VVR>CR+1 ?
	if err := x.checkRecoveryNeeded(vote.HighQc); err != nil {
		// we need to buffer the vote(s) so that when recovery succeeds we can "replay"
		// them - otherwise there might not be enough votes to achieve quorum and round
		// will time out
		x.voteBuffer[vote.Author] = vote
		err = fmt.Errorf("vote triggers recovery: %w", err)
		if e := x.sendRecoveryRequests(vote); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}

	qc, mature, err := x.pacemaker.RegisterVote(vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register vote: %w", err)
	}
	logger.Trace("%v round %v processed vote for round %v, quorum: %t, mature: %t",
		x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.VoteInfo.RoundNumber, qc != nil, mature)
	if qc != nil && mature {
		x.processQC(qc)
		x.processNewRoundEvent()
	}
	return nil
}

// onTimeoutMsg handles timeout vote messages from other root validators
// Timeout votes are broadcast to all nodes on local timeout and all validators try to assemble
// timeout certificate independently.
func (x *ConsensusManager) onTimeoutMsg(vote *abdrc.TimeoutMsg) error {
	if vote.Timeout.Round < x.pacemaker.GetCurrentRound() {
		return fmt.Errorf("stale timeout vote for round %d from %s", vote.Timeout.Round, vote.Author)
	}
	// verify signature on vote
	if err := vote.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers()); err != nil {
		return fmt.Errorf("invalid timeout vote: %w", err)
	}
	// SyncState, compare last handled QC
	if err := x.checkRecoveryNeeded(vote.Timeout.HighQc); err != nil {
		err = fmt.Errorf("timeout vote triggers recovery: %w", err)
		if e := x.sendRecoveryRequests(vote); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}
	tc, err := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register timeout vote: %w", err)
	}
	if tc == nil {
		logger.Trace("%v round %v processed timeout vote for round %v, no quorum yet",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), vote.Timeout.Round)
		return nil
	}
	logger.Debug("%v round %v timeout quorum achieved", x.id.ShortString(), vote.Timeout.Round)
	// process timeout certificate to advance to next the view/round
	x.processTC(tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.id {
		x.processNewRoundEvent()
	} else {
		logger.Trace("%v round %v, new leader is %v, waiting for proposal",
			x.id.ShortString(), x.pacemaker.GetCurrentRound(), l.String())
	}
	return nil
}

/*
checkRecoveryNeeded verify current state against received state and determine if the validator needs to
recover or not. Basically either the state is different or validator is behind (has skipped some views/rounds).
Returns nil when no recovery is needed and error describing the reason to trigger the recovery otherwise.
*/
func (x *ConsensusManager) checkRecoveryNeeded(qc *abtypes.QuorumCert) error {
	// when node has fallen behind we trigger recovery here as we fail to load block for
	// the round - it would be nicer to have explicit round check?
	rootHash, err := x.blockStore.GetBlockRootHash(qc.VoteInfo.RoundNumber)
	if err != nil {
		return fmt.Errorf("failed to read root hash for round %d from local block store: %w", qc.VoteInfo.RoundNumber, err)
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, rootHash) {
		return fmt.Errorf("unexpected round %d state - expected %X, local %X", qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, rootHash)
	}
	return nil
}

// onProposalMsg handles block proposal messages from other validators.
// Only a proposal made by the leader of this view/round shall be accepted and processed
func (x *ConsensusManager) onProposalMsg(ctx context.Context, proposal *abdrc.ProposalMsg) error {
	if proposal.Block.Round < x.pacemaker.GetCurrentRound() {
		return fmt.Errorf("stale proposal for round %d from %s", proposal.Block.Round, proposal.Block.Author)
	}
	// verify signature on proposal (does not verify partition request signatures)
	if err := proposal.Verify(x.trustBase.GetQuorumThreshold(), x.trustBase.GetVerifiers()); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	// Is from valid leader
	if l := x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String(); l != proposal.Block.Author {
		return fmt.Errorf("expected %s to be leader of the round %d but got proposal from %s", l, proposal.Block.Round, proposal.Block.Author)
	}
	// Check current state against new QC
	if err := x.checkRecoveryNeeded(proposal.Block.Qc); err != nil {
		err = fmt.Errorf("proposal triggers recovery: %w", err)
		if e := x.sendRecoveryRequests(proposal); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processQC(proposal.Block.Qc)
	x.processTC(proposal.LastRoundTc)
	// execute proposed payload
	execStateId, err := x.blockStore.Add(proposal.Block, x.irReqVerifier)
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
		return fmt.Errorf("failed to execute proposal: %w", err)
	}
	// make vote
	voteMsg, err := x.safety.MakeVote(proposal.Block, execStateId, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC())
	if err != nil {
		// wait for timeout, if others make progress this node will need to recover
		return fmt.Errorf("failed to sign vote: %w", err)
	}

	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	logger.Trace("%v sending vote to next leader %v", x.id.ShortString(), nextLeader.String())
	err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootVote,
			Message:  voteMsg,
		}, []peer.ID{nextLeader})
	if err != nil {
		return fmt.Errorf("failed to send vote to next leader: %w", err)
	}

	x.replayVoteBuffer()

	return nil
}

// processQC - handles quorum certificate
func (x *ConsensusManager) processQC(qc *abtypes.QuorumCert) {
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		logger.Warning("%v round %v failure to process QC triggers recovery: %v", x.id.ShortString(), x.pacemaker.GetCurrentRound(), err)
		if err := x.sendRecoveryRequests(qc); err != nil {
			logger.Warning("%v sending recovery requests failed: %v", x.id.ShortString(), err)
		}
		return
	}
	for _, uc := range certs {
		x.certResultCh <- uc
	}

	if !x.pacemaker.AdvanceRoundQC(qc) {
		return
	}

	// in the "DiemBFT v4" pseudo-code the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err := x.leaderSelector.Update(qc, x.pacemaker.GetCurrentRound()); err != nil {
		logger.Error("%v failed to update leader selector: %v", x.id.ShortString(), err)
	}
}

// processTC - handles timeout certificate
func (x *ConsensusManager) processTC(tc *abtypes.TimeoutCert) {
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		logger.Warning("%v failed to handle timeout certificate: %v", x.id.ShortString(), err)
	}
	x.pacemaker.AdvanceRoundTC(tc)
}

/*
replayVoteBuffer processes buffered votes. When method returns the buffer should be empty.
*/
func (x *ConsensusManager) replayVoteBuffer() {
	voteCnt := len(x.voteBuffer)
	if voteCnt == 0 {
		return
	}

	logger.Debug("%s round %d replaying %d buffered votes", x.id.ShortString(), x.pacemaker.GetCurrentRound(), voteCnt)
	var errs []error
	for _, v := range x.voteBuffer {
		if err := x.onVoteMsg(v); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		// log the error(s) rather than return them as failing to process buffered
		// votes is not critical from the callers POV but we want to have this info
		// for debugging
		logger.Warning("%s round %d out of %d buffered votes %d caused error on replay: %v", x.id.ShortString(), x.pacemaker.GetCurrentRound(), voteCnt, len(errs), errors.Join(errs...))
	}

	for k := range x.voteBuffer {
		delete(x.voteBuffer, k)
	}
}

// processNewRoundEvent handled new view event, is called when either QC or TC is reached locally and
// triggers a new round. If this node is the leader in the new view/round, then make a proposal otherwise
// wait for a proposal from a leader in this round/view
func (x *ConsensusManager) processNewRoundEvent() {
	round := x.pacemaker.GetCurrentRound()
	if l := x.leaderSelector.GetLeaderForRound(round); l != x.id {
		logger.Info("%v round %v new round start, not leader awaiting proposal from %s", x.id.ShortString(), round, l.ShortString())
		return
	}
	logger.Info("%v round %v new round start, node is leader", x.id.ShortString(), round)
	// find partitions with T2 timeouts
	timeoutIds, err := x.t2Timeouts.GetT2Timeouts(round)
	// NB! error here is not fatal, still make a proposal, hopefully the next node will generate timeout
	// requests for partitions this node failed to query
	if err != nil {
		logger.Warning("failed to check timeouts for some partitions, %v", err)
	}
	proposalMsg := &abdrc.ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    x.id.String(),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(round, timeoutIds),
			Qc:        x.blockStore.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err := x.safety.SignProposal(proposalMsg); err != nil {
		logger.Warning("%v failed to send proposal message, message signing failed: %v", x.id.ShortString(), err)
	}
	// broadcast proposal message (also to self)
	logger.Trace("%v broadcasting proposal msg", x.id.ShortString())
	if err := x.net.Broadcast(
		network.OutputMessage{
			Protocol: network.ProtocolRootProposal,
			Message:  proposalMsg}); err != nil {
		logger.Warning("%v failed to send proposal message, network error: %v", x.id.ShortString(), err)
	}
}

func (x *ConsensusManager) onStateReq(req *abdrc.GetStateMsg) error {
	if x.recovery != nil {
		return fmt.Errorf("node is in recovery state")
	}

	certs := x.blockStore.GetCertificates()
	ucs := make([]*types.UnicityCertificate, 0, len(certs))
	for _, c := range certs {
		ucs = append(ucs, c)
	}
	committedBlock := x.blockStore.GetRoot()
	pendingBlocks := x.blockStore.GetPendingBlocks()
	pending := make([]*abdrc.RecoveryBlock, len(pendingBlocks))
	for i, b := range pendingBlocks {
		pending[i] = &abdrc.RecoveryBlock{
			Block:    b.BlockData,
			Ir:       storage.ToRecoveryInputData(b.CurrentIR),
			Qc:       b.Qc,
			CommitQc: nil,
		}
	}
	respMsg := &abdrc.StateMsg{
		Certificates: ucs,
		CommittedHead: &abdrc.RecoveryBlock{
			Block:    committedBlock.BlockData,
			Ir:       storage.ToRecoveryInputData(committedBlock.CurrentIR),
			Qc:       committedBlock.Qc,
			CommitQc: committedBlock.CommitQc,
		},
		BlockNode: pending,
	}
	peerID, err := peer.Decode(req.NodeId)
	if err != nil {
		return fmt.Errorf("invalid receiver identifier %q: %w", req.NodeId, err)
	}
	if err = x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootStateResp,
			Message:  respMsg}, []peer.ID{peerID}); err != nil {
		return fmt.Errorf("failed to send state response message: %w", err)
	}
	return nil
}

func (x *ConsensusManager) onStateResponse(ctx context.Context, rsp *abdrc.StateMsg) error {
	logger.Trace("%v round %v received state response; %s", x.id.ShortString(), x.pacemaker.GetCurrentRound(), x.recovery)
	if x.recovery == nil {
		// we do send out multiple state recovery request so do not return error when we ignore the ones after successful recovery...
		return nil
	}
	if err := rsp.CanRecoverToRound(x.recovery.toRound, x.params.HashAlgorithm, x.trustBase.GetVerifiers()); err != nil {
		return fmt.Errorf("state message not suitable for recovery to round %d: %w", x.recovery.toRound, err)
	}

	if err := x.blockStore.UpdateCertificates(rsp.Certificates); err != nil {
		return fmt.Errorf("updating block store failed: %w", err)
	}
	if err := x.blockStore.RecoverState(rsp.CommittedHead, rsp.BlockNode, x.irReqVerifier); err != nil {
		return fmt.Errorf("recovering state in block store failed: %w", err)
	}

	x.pacemaker.Reset(x.blockStore.GetHighQc().GetRound())

	// exit recovery status and replay buffered messages
	logger.Debug("%v completed recovery to round %d", x.id.ShortString(), x.pacemaker.GetCurrentRound())
	triggerMsg := x.recovery.triggerMsg
	x.recovery = nil

	if prop, ok := triggerMsg.(*abdrc.ProposalMsg); ok {
		if err := x.onProposalMsg(ctx, prop); err != nil {
			logger.Debug("%s replaying proposal which triggered recovery returned error: %v", x.id.ShortString(), err)
		}
	}

	x.replayVoteBuffer()

	return nil
}

func (x *ConsensusManager) sendRecoveryRequests(triggerMsg any) error {
	info, signatures, err := msgToRecoveryInfo(triggerMsg)
	if err != nil {
		return fmt.Errorf("failed to extract recovery info: %w", err)
	}
	if x.recovery != nil && x.recovery.toRound >= info.toRound {
		return fmt.Errorf("already in recovery to round %d, ignoring request to recover to round %d", x.recovery.toRound, info.toRound)
	}

	// set recovery status - when send fails we won't check did we fail to send to just one peer or to
	// all of them... if we completely failed to send we'll (re)enter to recovery status with higher
	// destination round when receiving proposal or vote for the next round...
	x.recovery = info

	if err := x.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolRootStateReq,
			Message:  &abdrc.GetStateMsg{NodeId: x.id.String()}},
		selectRandomNodeIdsFromSignatureMap(signatures, 2)); err != nil {
		return fmt.Errorf("failed to send recovery request: %w", err)
	}
	return nil
}

func msgToRecoveryInfo(msg any) (info *recoveryInfo, signatures map[string][]byte, err error) {
	info = &recoveryInfo{triggerMsg: msg}

	switch mt := msg.(type) {
	case *abdrc.ProposalMsg:
		info.toRound = mt.Block.Qc.GetRound()
		signatures = mt.Block.Qc.Signatures
	case *abdrc.VoteMsg:
		info.toRound = mt.HighQc.GetRound()
		signatures = mt.HighQc.Signatures
	case *abdrc.TimeoutMsg:
		info.toRound = mt.Timeout.HighQc.VoteInfo.RoundNumber
		signatures = mt.Timeout.HighQc.Signatures
	case *abtypes.QuorumCert:
		info.toRound = mt.GetParentRound()
		signatures = mt.Signatures
	default:
		return nil, nil, fmt.Errorf("unknown message type, cannot be used for recovery: %T", mt)
	}

	return info, signatures, nil
}

/*
selectRandomNodeIdsFromSignatureMap returns slice with up to "count" random keys
from "m" without duplicates. The "count" assumed to be greater than zero, iow the
function always returns at least one item (given that map is not empty).
The key of the "m" must be of type peer.ID encoded as string (if it's not it is ignored).
When "m" has less items than "count" then len(m) items is returned (iow all map keys),
when "m" is empty then empty/nil slice is returned.
*/
func selectRandomNodeIdsFromSignatureMap(m map[string][]byte, count int) (nodes []peer.ID) {
	for k := range m {
		id, err := peer.Decode(k)
		if err != nil {
			continue
		}
		if slices.Contains(nodes, id) {
			continue
		}
		nodes = append(nodes, id)
		if count--; count == 0 {
			return nodes
		}
	}
	return nodes
}

func (ri *recoveryInfo) String() string {
	if ri == nil {
		return "<nil>"
	}
	return fmt.Sprintf("toRound: %d trigger %T", ri.toRound, ri.triggerMsg)
}
