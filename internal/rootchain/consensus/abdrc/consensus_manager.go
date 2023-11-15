package abdrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/leader"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

// how long to wait before repeating status request
const statusReqShelfLife = 800 * time.Millisecond

type (
	recoveryInfo struct {
		toRound    uint64
		triggerMsg any
		sent       time.Time // when the status request was created/sent
	}

	RootNet interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any
	}

	// Leader provides interface to different leader selection algorithms
	Leader interface {
		// GetLeaderForRound returns valid leader (node id) for round/view number
		GetLeaderForRound(round uint64) peer.ID
		// GetNodes - get all node id's currently active
		GetNodes() []peer.ID
		// Update - what PaceMaker considers to be the current round at the time QC is processed.
		Update(qc *abtypes.QuorumCert, currentRound uint64) error
	}

	ConsensusManager struct {
		// channel via which validator sends "certification requests" to CM
		certReqCh chan consensus.IRChangeRequest
		// channel via which CM sends "certification request" response to validator
		certResultCh chan *types.UnicityCertificate
		// internal buffer for "certification request" response to allow CM to
		// continue without waiting validator to consume the response
		ucSink         chan map[types.SystemID32]*types.UnicityCertificate
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
		log      *slog.Logger
	}
)

// NewDistributedAbConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewDistributedAbConsensusManager(nodeID peer.ID, rg *genesis.RootGenesis,
	partitionStore partitions.PartitionConfiguration, net RootNet, signer crypto.Signer, log *slog.Logger, opts ...consensus.Option) (*ConsensusManager, error) {
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
		ucSink:         make(chan map[types.SystemID32]*types.UnicityCertificate, 1),
		params:         cParams,
		id:             nodeID,
		net:            net,
		pacemaker:      NewPacemaker(cParams.BlockRate/2, cParams.LocalTimeout),
		leaderSelector: leader,
		trustBase:      tb,
		irReqBuffer:    NewIrReqBuffer(log),
		safety:         safetyModule,
		blockStore:     bStore,
		partitions:     partitionStore,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
		voteBuffer:     make(map[string]*abdrc.VoteMsg),
		log:            log,
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

func (x *ConsensusManager) GetLatestUnicityCertificate(id types.SystemID32) (*types.UnicityCertificate, error) {
	return x.blockStore.GetCertificate(id)
}

func (x *ConsensusManager) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer x.pacemaker.Stop()
		x.pacemaker.Reset(x.blockStore.GetHighQc().VoteInfo.RoundNumber)
		currentRound := x.pacemaker.GetCurrentRound()
		x.log.InfoContext(ctx, fmt.Sprintf("CM starting, leader is %s", x.leaderSelector.GetLeaderForRound(currentRound)), logger.Round(currentRound))
		return x.loop(ctx)
	})

	g.Go(func() error { return x.sendCertificates(ctx) })

	err := g.Wait()
	x.log.InfoContext(ctx, "exited distributed consensus manager main loop", logger.Error(err))
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
			x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("received %T", msg), logger.Data(msg), logger.Round(x.pacemaker.GetCurrentRound()))
			if err := x.handleRootNetMsg(ctx, msg); err != nil {
				x.log.WarnContext(ctx, fmt.Sprintf("processing %T", msg), logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
			}
		case req := <-x.certReqCh:
			if err := x.onPartitionIRChangeReq(ctx, &req); err != nil {
				x.log.WarnContext(ctx, "failed to process IR change request from partition", logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
			}
		case event := <-x.pacemaker.StatusEvents():
			switch event {
			case pmsRoundMatured:
				currentRound := x.pacemaker.GetCurrentRound()
				leader := x.leaderSelector.GetLeaderForRound(currentRound + 1)
				x.log.DebugContext(ctx, fmt.Sprintf("round has lasted minimum required duration; next leader %s", leader.ShortString()), logger.Round(currentRound))
				// round 2 is system bootstrap and is a special case - as there is no proposal no one is sending votes
				// and thus leader won't achieve quorum and doesn't make next proposal (and the round would time out).
				// So we just have the round 2 leader to trigger next round when it's mature (root genesis QC will be
				// used as HighQc in the proposal).
				if leader == x.id || (currentRound == 2 && x.id == x.leaderSelector.GetLeaderForRound(2)) {
					if qc := x.pacemaker.RoundQC(); qc != nil || currentRound == 2 {
						x.processQC(ctx, qc)
						x.processNewRoundEvent(ctx)
					}
				}
			case pmsRoundTimeout:
				x.onLocalTimeout(ctx)
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
	case *abdrc.IrChangeReqMsg:
		return x.onIRChangeMsg(ctx, mt)
	case *abdrc.ProposalMsg:
		return x.onProposalMsg(ctx, mt)
	case *abdrc.VoteMsg:
		return x.onVoteMsg(ctx, mt)
	case *abdrc.TimeoutMsg:
		return x.onTimeoutMsg(ctx, mt)
	case *abdrc.GetStateMsg:
		return x.onStateReq(ctx, mt)
	case *abdrc.StateMsg:
		return x.onStateResponse(ctx, mt)
	}
	return fmt.Errorf("unknown message type %T", msg)
}

// onLocalTimeout handle local timeout, either no proposal is received or voting does not
// reach consensus. Triggers timeout voting.
func (x *ConsensusManager) onLocalTimeout(ctx context.Context) {
	x.log.InfoContext(ctx, "local timeout", logger.Round(x.pacemaker.GetCurrentRound()))
	// has the validator voted in this round, if true send the same (timeout)vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = abdrc.NewTimeoutMsg(
			abtypes.NewTimeout(x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC()),
			x.id.String())
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			x.log.WarnContext(ctx, "signing timeout", logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
			return
		}
		x.pacemaker.SetTimeoutVote(timeoutVoteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	x.log.LogAttrs(ctx, logger.LevelTrace, "broadcasting timeout vote", logger.Round(x.pacemaker.GetCurrentRound()))
	if err := x.net.Send(ctx, timeoutVoteMsg, x.leaderSelector.GetNodes()...); err != nil {
		x.log.WarnContext(ctx, "error on broadcasting timeout vote", logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
	}
}

// onPartitionIRChangeReq handle partition change requests. Received from go routine handling
// partition communication when either partition reaches consensus or cannot reach consensus.
func (x *ConsensusManager) onPartitionIRChangeReq(ctx context.Context, req *consensus.IRChangeRequest) error {
	x.log.DebugContext(ctx, "IR change request from partition", logger.Round(x.pacemaker.GetCurrentRound()))
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
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	if nextLeader == x.id {
		x.log.LogAttrs(ctx, slog.LevelDebug, "node is the next leader, add to buffer")
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irReq, x.irReqVerifier); err != nil {
			return fmt.Errorf("failed to add IR change request into buffer: %w", err)
		}
		return nil
	}
	// forward to leader
	irMsg := &abdrc.IrChangeReqMsg{
		Author:      x.id.String(),
		IrChangeReq: irReq,
	}
	if err := x.safety.Sign(irMsg); err != nil {
		return fmt.Errorf("failed to sign ir change request message, %w", err)
	}
	x.log.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("forward ir change request to %s", nextLeader.ShortString()))
	if err := x.net.Send(ctx, irMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to send ir change request message, %w", err)
	}
	return nil
}

// onIRChangeMsg handles IR change request messages from other root nodes
func (x *ConsensusManager) onIRChangeMsg(ctx context.Context, irChangeMsg *abdrc.IrChangeReqMsg) error {
	x.log.DebugContext(ctx, "IR change request from root node", logger.Round(x.pacemaker.GetCurrentRound()))
	if err := irChangeMsg.Verify(x.trustBase.GetVerifiers()); err != nil {
		return fmt.Errorf("invalid IR change request message from node %s: %w", irChangeMsg.Author, err)
	}
	x.log.DebugContext(ctx, fmt.Sprintf("IR change request from node %s",
		irChangeMsg.Author), logger.Round(x.pacemaker.GetCurrentRound()))
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// if the node will be the next leader then buffer the request to be included in the block proposal
	// todo: if in recovery then forward to next?
	if nextLeader == x.id {
		x.log.LogAttrs(ctx, slog.LevelDebug, "node is the next leader, add to buffer")
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChangeMsg.IrChangeReq, x.irReqVerifier); err != nil {
			return fmt.Errorf("failed to add IR change request into buffer: %w", err)
		}
		return nil
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this is a completely lost message or because of race we just proposed, forward the original
	// message again to next leader
	x.log.WarnContext(ctx, "node is not the leader in the next round, forwarding again", logger.Round(x.pacemaker.GetCurrentRound()))
	if err := x.net.Send(ctx, irChangeMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to forward IR change message to the next leader: %w", err)
	}
	return nil
}

// onVoteMsg handle votes messages from other root validators
func (x *ConsensusManager) onVoteMsg(ctx context.Context, vote *abdrc.VoteMsg) error {
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
		x.log.DebugContext(ctx, "received vote before proposal, buffering vote", logger.Round(x.pacemaker.GetCurrentRound()))
		x.voteBuffer[vote.Author] = vote
		// if we have received quorum votes, but no proposal yet, then try and recover the proposal.
		// NB! it seems that it's quite common that votes arrive before proposal and going into recovery
		// too early is counterproductive... maybe do not trigger recovery here at all - if we're lucky
		// proposal will arrive on time, otherwise round will likely TO anyway?
		if uint32(len(x.voteBuffer)) >= x.trustBase.GetQuorumThreshold() {
			err = fmt.Errorf("have received %d votes but no proposal, entering recovery", len(x.voteBuffer))
			if e := x.sendRecoveryRequests(ctx, vote); e != nil {
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
		if e := x.sendRecoveryRequests(ctx, vote); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}

	qc, mature, err := x.pacemaker.RegisterVote(vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register vote: %w", err)
	}
	x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("processed vote, quorum: %t, mature: %t", qc != nil, mature), logger.Round(x.pacemaker.GetCurrentRound()))
	if qc != nil && mature {
		x.processQC(ctx, qc)
		x.processNewRoundEvent(ctx)
	}
	return nil
}

// onTimeoutMsg handles timeout vote messages from other root validators
// Timeout votes are broadcast to all nodes on local timeout and all validators try to assemble
// timeout certificate independently.
func (x *ConsensusManager) onTimeoutMsg(ctx context.Context, vote *abdrc.TimeoutMsg) error {
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
		if e := x.sendRecoveryRequests(ctx, vote); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}
	// node is up-to-date, first handle high QC, maybe this has not been seen yet
	x.processQC(ctx, vote.Timeout.HighQc)
	// when there is multiple consecutive timeout rounds and instance is in one of the previous round
	// (ie haven't got enough timeout votes for the latest round quorum) recovery is not triggered as
	// the highQC is the same for both rounds. So checking the lastTC helps the instance into latest TO round.
	// AdvanceRoundTC handles nil TC gracefully.
	x.processTC(vote.Timeout.LastTC)

	tc, err := x.pacemaker.RegisterTimeoutVote(vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register timeout vote: %w", err)
	}
	if tc == nil {
		x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("processed timeout vote for round %v, no quorum yet", vote.Timeout.Round), logger.Round(x.pacemaker.GetCurrentRound()))
		return nil
	}
	x.log.DebugContext(ctx, "timeout quorum achieved", logger.Round(vote.Timeout.Round))
	// process timeout certificate to advance to next the view/round
	x.processTC(tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.id {
		x.processNewRoundEvent(ctx)
	} else {
		x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("new leader is %s, waiting for proposal", l.String()), logger.Round(x.pacemaker.GetCurrentRound()))
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
		if e := x.sendRecoveryRequests(ctx, proposal); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processQC(ctx, proposal.Block.Qc)
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
	x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("sending vote to next leader %s", nextLeader.String()), logger.Round(proposal.Block.Round))
	if err = x.net.Send(ctx, voteMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to send vote to next leader: %w", err)
	}
	x.replayVoteBuffer(ctx)

	return nil
}

// processQC - handles quorum certificate
func (x *ConsensusManager) processQC(ctx context.Context, qc *abtypes.QuorumCert) {
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		x.log.WarnContext(ctx, "failure to process QC triggers recovery", logger.Round(x.pacemaker.GetCurrentRound()), logger.Error(err))
		if err := x.sendRecoveryRequests(ctx, qc); err != nil {
			x.log.WarnContext(ctx, "sending recovery requests failed", logger.Round(x.pacemaker.GetCurrentRound()), logger.Error(err))
		}
		return
	}
	x.ucSink <- certs

	if !x.pacemaker.AdvanceRoundQC(qc) {
		return
	}

	// in the "DiemBFT v4" pseudo-code the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err := x.leaderSelector.Update(qc, x.pacemaker.GetCurrentRound()); err != nil {
		x.log.ErrorContext(ctx, "failed to update leader selector", logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
	}
}

// processTC - handles timeout certificate
func (x *ConsensusManager) processTC(tc *abtypes.TimeoutCert) {
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		// method deletes the block that got TC - it will never be part of the chain.
		// however, this node might not have even seen the block, in which case error is returned, but this is ok - just log
		x.log.Debug("could not remove the timeout block, node has not received it", logger.Error(err))
	}
	x.pacemaker.AdvanceRoundTC(tc)
}

/*
sendCertificates reads UCs produced by processing QC and makes them available for
validator via certResultCh chan (returned by CertificationResult method).
The idea is not to block CM until validator consumes the certificates, ie to
send the UCs in a async fashion.
*/
func (x *ConsensusManager) sendCertificates(ctx context.Context) error {
	// pending certificates, to be consumed by the validator.
	// access to it is "serialized" ie we either update it with
	// new certs sent by CM or we feed it's content to validator
	certs := make(map[types.SystemID32]*types.UnicityCertificate)

	feedValidator := func(ctx context.Context) chan struct{} {
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			for sysID, uc := range certs {
				select {
				case x.certResultCh <- uc:
					delete(certs, sysID)
				case <-ctx.Done():
					return
				}
			}
		}()
		return stopped
	}

	stopFeed := func() { /* init to NOP */ }
	for {
		select {
		case nm := <-x.ucSink:
			stopFeed()
			// NB! if previous UC for given system hasn't been consumed yet we'll overwrite it!
			// this means that the validator sees newer UC than expected and goes into recovery,
			// rolling back pending block proposal?
			for sysID, uc := range nm {
				certs[sysID] = uc
			}
			feedCtx, cancel := context.WithCancel(ctx)
			stopped := feedValidator(feedCtx)
			stopFeed = func() {
				cancel()
				<-stopped
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

/*
replayVoteBuffer processes buffered votes. When method returns the buffer should be empty.
*/
func (x *ConsensusManager) replayVoteBuffer(ctx context.Context) {
	voteCnt := len(x.voteBuffer)
	if voteCnt == 0 {
		return
	}

	x.log.DebugContext(ctx, fmt.Sprintf("replaying %d buffered votes", voteCnt), logger.Round(x.pacemaker.GetCurrentRound()))
	var errs []error
	for _, v := range x.voteBuffer {
		if err := x.onVoteMsg(ctx, v); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		// log the error(s) rather than return them as failing to process buffered
		// votes is not critical from the callers POV but we want to have this info
		// for debugging
		x.log.WarnContext(ctx, fmt.Sprintf("out of %d buffered votes %d caused error on replay", voteCnt, len(errs)), logger.Round(x.pacemaker.GetCurrentRound()), logger.Error(errors.Join(errs...)))
	}

	clear(x.voteBuffer)
}

// processNewRoundEvent handled new view event, is called when either QC or TC is reached locally and
// triggers a new round. If this node is the leader in the new view/round, then make a proposal otherwise
// wait for a proposal from a leader in this round/view
func (x *ConsensusManager) processNewRoundEvent(ctx context.Context) {
	round := x.pacemaker.GetCurrentRound()
	if l := x.leaderSelector.GetLeaderForRound(round); l != x.id {
		x.log.InfoContext(ctx, fmt.Sprintf("new round start, not leader, awaiting proposal from %s", l.ShortString()), logger.Round(round))
		return
	}
	x.log.InfoContext(ctx, "new round start, node is leader", logger.Round(round))
	// find partitions with T2 timeouts
	timeoutIds, err := x.t2Timeouts.GetT2Timeouts(round)
	// NB! error here is not fatal, still make a proposal, hopefully the next node will generate timeout
	// requests for partitions this node failed to query
	if err != nil {
		x.log.WarnContext(ctx, "failed to check timeouts for some partitions", logger.Error(err), logger.Round(round))
	}
	proposalMsg := &abdrc.ProposalMsg{
		Block: &abtypes.BlockData{
			Author:    x.id.String(),
			Round:     round,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload: x.irReqBuffer.GeneratePayload(round, timeoutIds, func(id types.SystemID32) bool {
				return x.blockStore.IsChangeInProgress(id)
			}),
			Qc: x.blockStore.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err = x.safety.Sign(proposalMsg); err != nil {
		x.log.WarnContext(ctx, "failed to send proposal message, message signing failed", logger.Error(err), logger.Round(round))
	}
	// broadcast proposal message (also to self)
	x.log.LogAttrs(ctx, slog.LevelDebug, "broadcast proposal", logger.Data(proposalMsg.Block.String()), logger.Round(round))
	if err = x.net.Send(ctx, proposalMsg, x.leaderSelector.GetNodes()...); err != nil {
		x.log.WarnContext(ctx, "failed to send proposal message", logger.Error(err), logger.Round(round))
	}
}

func (x *ConsensusManager) onStateReq(ctx context.Context, req *abdrc.GetStateMsg) error {
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
	if err = x.net.Send(ctx, respMsg, peerID); err != nil {
		return fmt.Errorf("failed to send state response message: %w", err)
	}
	return nil
}

func (x *ConsensusManager) onStateResponse(ctx context.Context, rsp *abdrc.StateMsg) error {
	x.log.LogAttrs(ctx, logger.LevelTrace, "received state response", logger.Round(x.pacemaker.GetCurrentRound()), logger.Data(x.recovery))
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
	x.log.DebugContext(ctx, "completed recovery", logger.Round(x.pacemaker.GetCurrentRound()))
	triggerMsg := x.recovery.triggerMsg
	x.recovery = nil

	if prop, ok := triggerMsg.(*abdrc.ProposalMsg); ok {
		if err := x.onProposalMsg(ctx, prop); err != nil {
			x.log.DebugContext(ctx, "replaying proposal which triggered recovery returned error", logger.Error(err), logger.Round(x.pacemaker.GetCurrentRound()))
		}
	}

	x.replayVoteBuffer(ctx)

	return nil
}

func (x *ConsensusManager) sendRecoveryRequests(ctx context.Context, triggerMsg any) error {
	info, signatures, err := msgToRecoveryInfo(triggerMsg)
	if err != nil {
		return fmt.Errorf("failed to extract recovery info: %w", err)
	}
	if x.recovery != nil && x.recovery.toRound >= info.toRound && time.Since(x.recovery.sent) < statusReqShelfLife {
		return fmt.Errorf("already in recovery to round %d, ignoring request to recover to round %d", x.recovery.toRound, info.toRound)
	}

	// set recovery status - when send fails we won't check did we fail to send to just one peer or
	// to all of them... if we completely failed to send (or recovery just fails) we'll (re)enter to
	// recovery status with higher destination round (when receiving proposal or vote for the next
	// round) or after current request "times out"...
	x.recovery = info

	if err = x.net.Send(ctx, &abdrc.GetStateMsg{NodeId: x.id.String()},
		selectRandomNodeIdsFromSignatureMap(signatures, 2)...); err != nil {
		return fmt.Errorf("failed to send recovery request: %w", err)
	}
	return nil
}

func msgToRecoveryInfo(msg any) (info *recoveryInfo, signatures map[string][]byte, err error) {
	info = &recoveryInfo{triggerMsg: msg, sent: time.Now()}

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
	return fmt.Sprintf("toRound: %d trigger %T @ %s", ri.toRound, ri.triggerMsg, ri.sent)
}
