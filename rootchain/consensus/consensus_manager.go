package consensus

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus/leader"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
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
		Update(qc *drctypes.QuorumCert, currentRound uint64, b leader.BlockLoader) error
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Tracer(name string, options ...trace.TracerOption) trace.Tracer
		Logger() *slog.Logger
		RoundLogger(curRound func() uint64) *slog.Logger
	}

	Orchestration interface {
		NetworkID() types.NetworkID
		ShardConfig(partitionID types.PartitionID, shardID types.ShardID, rootRound uint64) (*types.PartitionDescriptionRecord, error)
		ShardConfigs(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error)
	}

	certRequest struct {
		ircr IRChangeRequest
		rsc  trace.SpanContext
	}

	ConsensusManager struct {
		// channel via which validator sends "certification requests" to CM
		certReqCh chan certRequest
		// channel via which CM sends "certification request" response to validator
		certResultCh chan *certification.CertificationResponse
		// internal buffer for "certification request" response to allow CM to
		// continue without waiting validator to consume the response
		ucSink         chan []*certification.CertificationResponse
		params         *Parameters
		id             peer.ID
		net            RootNet
		pacemaker      *Pacemaker
		leaderSelector Leader
		trustBase      types.RootTrustBase
		irReqBuffer    *IrReqBuffer
		safety         *SafetyModule
		blockStore     *storage.BlockStore
		orchestration  Orchestration
		irReqVerifier  *IRChangeReqVerifier
		t2Timeouts     *PartitionTimeoutGenerator
		// votes need to be buffered when CM will be the next leader (so other nodes
		// will send votes to it) but it hasn't got the proposal yet, so it can't process
		// the votes. voteBuffer maps author id to vote, so we do not buffer same vote
		// multiple times (votes are buffered for single round only)
		voteBuffer map[string]*abdrc.VoteMsg
		// whether the CM is in recovery mode, trying to get into the same state as other CMs
		recovery *recoveryState

		log    *slog.Logger
		tracer trace.Tracer

		leaderCnt   metric.Int64Counter
		execMsgCnt  metric.Int64Counter
		execMsgDur  metric.Float64Histogram
		voteCnt     metric.Int64Counter
		proposedCR  metric.Int64Counter
		fwdIRCRCnt  metric.Int64Counter
		qcSize      metric.Int64Counter
		qcVoters    metric.Int64Counter
		susVotes    metric.Int64Counter
		recoveryReq metric.Int64Counter
		addBlockDur metric.Float64Histogram
	}
)

// NewConsensusManager creates new "Atomic Broadcast" protocol based distributed consensus manager
func NewConsensusManager(
	nodeID peer.ID,
	trustBase types.RootTrustBase,
	orchestration Orchestration,
	net RootNet,
	signer abcrypto.Signer,
	observe Observability,
	opts ...Option,
) (*ConsensusManager, error) {
	// Sanity checks
	if net == nil {
		return nil, errors.New("network is nil")
	}
	if trustBase == nil {
		return nil, errors.New("trust base is nil")
	}
	// load options
	optional, err := LoadConf(opts)
	if err != nil {
		return nil, fmt.Errorf("loading optional configuration: %w", err)
	}

	cParams := optional.Params
	pm, err := NewPacemaker(cParams.BlockRate/2, cParams.LocalTimeout, observe)
	if err != nil {
		return nil, fmt.Errorf("creating Pacemaker: %w", err)
	}
	log := observe.RoundLogger(pm.GetCurrentRound)

	// init storage
	bStore, err := storage.New(cParams.HashAlgorithm, optional.Storage, orchestration, log)
	if err != nil {
		return nil, fmt.Errorf("consensus block storage init failed: %w", err)
	}
	reqVerifier, err := NewIRChangeReqVerifier(cParams, bStore)
	if err != nil {
		return nil, fmt.Errorf("block verifier construct error: %w", err)
	}
	t2TimeoutGen, err := NewLucBasedT2TimeoutGenerator(cParams, bStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create T2 timeout generator: %w", err)
	}
	safetyModule, err := NewSafetyModule(trustBase.GetNetworkID(), nodeID.String(), signer, optional.Storage)
	if err != nil {
		return nil, err
	}
	ls, err := leaderSelector(trustBase)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus leader selector: %w", err)
	}
	consensusManager := &ConsensusManager{
		certReqCh:      make(chan certRequest),
		certResultCh:   make(chan *certification.CertificationResponse),
		ucSink:         make(chan []*certification.CertificationResponse, 1),
		params:         cParams,
		id:             nodeID,
		net:            net,
		pacemaker:      pm,
		leaderSelector: ls,
		trustBase:      trustBase,
		irReqBuffer:    NewIrReqBuffer(log),
		safety:         safetyModule,
		blockStore:     bStore,
		orchestration:  orchestration,
		irReqVerifier:  reqVerifier,
		t2Timeouts:     t2TimeoutGen,
		voteBuffer:     make(map[string]*abdrc.VoteMsg),
		recovery:       &recoveryState{},
		log:            log,
		tracer:         observe.Tracer("cm.distributed"),
	}
	if err := consensusManager.initMetrics(observe); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}
	return consensusManager, nil
}

func (x *ConsensusManager) initMetrics(observe Observability) (err error) {
	m := observe.Meter("cm.distributed")

	_, err = m.Int64ObservableCounter("round", metric.WithDescription("current round"),
		metric.WithInt64Callback(func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(int64(x.pacemaker.GetCurrentRound()))
			return nil
		}))
	if err != nil {
		return fmt.Errorf("creating counter for round number: %w", err)
	}

	x.leaderCnt, err = m.Int64Counter("round.leader", metric.WithDescription("Number of times node has been round leader"))
	if err != nil {
		return fmt.Errorf("creating counter for leader count: %w", err)
	}
	x.voteCnt, err = m.Int64Counter("count.vote", metric.WithDescription("Number of times node has voted (might be more than once per round for a different reason, ie proposal and timeout)"))
	if err != nil {
		return fmt.Errorf("creating vote counter: %w", err)
	}
	x.susVotes, err = m.Int64Counter("sus.vote", metric.WithDescription(`Number of "suspicious" votes node has seen (ie stale votes, votes before proposal, etc)`))
	if err != nil {
		return fmt.Errorf("creating suspicious vote counter: %w", err)
	}

	x.proposedCR, err = m.Int64Counter("count.proposed.cr", metric.WithDescription("Number of Change Requests included into proposal by the round leader"))
	if err != nil {
		return fmt.Errorf("creating counter for proposal change requests count: %w", err)
	}
	x.fwdIRCRCnt, err = m.Int64Counter("count.fwd.ircr", metric.WithDescription(`Number of IR Change Requests messages forwarded ("lost" messages)`))
	if err != nil {
		return fmt.Errorf("creating counter for proposal change requests count: %w", err)
	}

	x.execMsgCnt, err = m.Int64Counter("exec.msg.count", metric.WithDescription("Number of messages processed by the consensus manager"))
	if err != nil {
		return fmt.Errorf("creating counter for processed messages: %w", err)
	}
	x.execMsgDur, err = m.Float64Histogram("exec.msg.time",
		metric.WithDescription("How long it took to process message"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(100e-6, 200e-6, 400e-6, 800e-6, 0.0016, 0.01, 0.05, 0.1))
	if err != nil {
		return fmt.Errorf("creating histogram for processed messages: %w", err)
	}

	x.addBlockDur, err = m.Float64Histogram("add.block.time",
		metric.WithDescription("How long it took to add block from proposal"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128))
	if err != nil {
		return fmt.Errorf("creating histogram for adding block: %w", err)
	}

	x.qcSize, err = m.Int64Counter("qc.vote.count", metric.WithDescription("Number of votes in the quorum certificate"))
	if err != nil {
		return fmt.Errorf("creating counter for votes in QC: %w", err)
	}
	x.qcVoters, err = m.Int64Counter("qc.participated", metric.WithDescription("Number of times node participated in the QC (vote was included)"))
	if err != nil {
		return fmt.Errorf("creating counter for votes by node included into QC: %w", err)
	}

	x.recoveryReq, err = m.Int64Counter("recovery", metric.WithDescription("Number of times node has entered into recovery state and sent out state request"))
	if err != nil {
		return fmt.Errorf("creating counter for recovery attempts: %w", err)
	}

	return nil
}

func leaderSelector(trustBase types.RootTrustBase) (ls Leader, err error) {
	// NB! both leader selector algorithms make the assumption that the rootNodes slice is
	// sorted, and it's content doesn't change!
	validators, err := toPeerIDs(trustBase.GetRootNodes())
	if err != nil {
		return nil, fmt.Errorf("failed to get root validator peerIDs: %w", err)
	}

	slices.Sort(validators)

	switch len(validators) {
	case 0:
		return nil, errors.New("number of peers must be greater than zero")
	case 1:
		// really should have "constant leader" algorithm but this shouldn't happen IRL.
		// we need this case as some tests create only one root node. Could use reputation
		// based selection with excludeSize=0 but round-robin is more efficient...
		return leader.NewRoundRobin(validators, 1)
	default:
		// we're limited to window size and exclude size 1 as our block loader (block store) doesn't
		// keep history, ie we can't load blocks older than previous block.
		return leader.NewReputationBased(validators, 1, 1)
	}
}

func toPeerIDs(nodes []*types.NodeInfo) ([]peer.ID, error) {
	peerIDs := make([]peer.ID, len(nodes))
	for n, v := range nodes {
		peerID, err := peer.Decode(v.NodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to convert node ID %q: %w", v.NodeID, err)
		}
		peerIDs[n] = peerID
	}
	return peerIDs, nil
}

func (x *ConsensusManager) RequestCertification(ctx context.Context, cr IRChangeRequest) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.RequestCertification")
	defer span.End()

	if x.recovery.InRecovery() {
		return fmt.Errorf("node is in recovery: %s", x.recovery)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case x.certReqCh <- certRequest{ircr: cr, rsc: trace.SpanContextFromContext(ctx)}:
	}
	return nil
}

func (x *ConsensusManager) CertificationResult() <-chan *certification.CertificationResponse {
	return x.certResultCh
}

func (x *ConsensusManager) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer x.pacemaker.Stop()
		vote, err := x.blockStore.ReadLastVote()
		if err != nil {
			return fmt.Errorf("last vote read failed: %w", err)
		}
		hQc := x.blockStore.GetHighQc()
		lastTC, err := x.blockStore.GetLastTC()
		if err != nil {
			return fmt.Errorf("failed to read last TC from block store: %w", err)
		}
		x.pacemaker.Reset(ctx, hQc.GetRound(), lastTC, vote)
		x.log.InfoContext(ctx, fmt.Sprintf("CM starting, leader is %s", x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())))
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
			x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("received %T", msg), logger.Data(msg))
			if err := x.handleRootNetMsg(ctx, msg); err != nil {
				x.log.WarnContext(ctx, fmt.Sprintf("processing %T", msg), logger.Error(err))
			}
		case req := <-x.certReqCh:
			ctx := trace.ContextWithRemoteSpanContext(ctx, req.rsc)
			if err := x.onPartitionIRChangeReq(ctx, &req.ircr); err != nil {
				x.log.WarnContext(ctx, "failed to process IR change request from partition", logger.Error(err), logger.Shard(req.ircr.Partition, req.ircr.Shard))
			}
		case event := <-x.pacemaker.StatusEvents():
			x.handlePacemakerEvent(ctx, event)
		}
	}
}

/*
handleRootNetMsg routes messages from "root net" iow messages sent by other rootchain
validators to appropriate message handler.
*/
func (x *ConsensusManager) handleRootNetMsg(ctx context.Context, msg any) (rErr error) {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.handleRootNetMsg", trace.WithNewRoot(), trace.WithAttributes(observability.Round(x.pacemaker.GetCurrentRound())), trace.WithSpanKind(trace.SpanKindServer))
	defer func(start time.Time) {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		msgAttr := attribute.String("msg", fmt.Sprintf("%T", msg))
		x.execMsgCnt.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(msgAttr, observability.ErrStatus(rErr))))
		x.execMsgDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(msgAttr))
		span.SetAttributes(msgAttr)
		span.End()
	}(time.Now())

	switch mt := msg.(type) {
	case *abdrc.IrChangeReqMsg:
		return x.onIRChangeMsg(ctx, mt)
	case *abdrc.ProposalMsg:
		return x.onProposalMsg(ctx, mt)
	case *abdrc.VoteMsg:
		return x.onVoteMsg(ctx, mt)
	case *abdrc.TimeoutMsg:
		return x.onTimeoutMsg(ctx, mt)
	case *abdrc.StateRequestMsg:
		return x.onStateReq(ctx, mt)
	case *abdrc.StateMsg:
		return x.onStateResponse(ctx, mt)
	}
	return fmt.Errorf("unknown message type %T", msg)
}

func (x *ConsensusManager) handlePacemakerEvent(ctx context.Context, event paceMakerStatus) {
	currentRound := x.pacemaker.GetCurrentRound()
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.handlePacemakerEvent", trace.WithNewRoot(), trace.WithAttributes(observability.Round(currentRound), attribute.Stringer("event", event)))
	defer span.End()

	switch event {
	case pmsRoundMatured:
		nextLeader := x.leaderSelector.GetLeaderForRound(currentRound + 1)
		x.log.DebugContext(ctx, fmt.Sprintf("round has lasted minimum required duration; next leader %s", nextLeader.ShortString()))
		// round 2 is system bootstrap and is a special case - as there is no proposal no one is sending votes
		// and thus leader won't achieve quorum and doesn't make next proposal (and the round would time out).
		// So we just have the round 2 leader to trigger next round when it's mature (root genesis QC will be
		// used as HighQc in the proposal).
		if nextLeader == x.id || (currentRound == 2 && x.id == x.leaderSelector.GetLeaderForRound(2)) {
			if qc := x.pacemaker.RoundQC(); qc != nil || currentRound == 2 {
				x.processQC(ctx, qc)
				x.processNewRoundEvent(ctx)
				x.updateQCMetrics(ctx, qc)
			}
		}
	case pmsRoundTimeout:
		x.onLocalTimeout(ctx)
	}
}

// onLocalTimeout handle local timeout, either no proposal is received or voting does not
// reach consensus. Triggers timeout voting.
func (x *ConsensusManager) onLocalTimeout(ctx context.Context) {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onLocalTimeout")
	defer span.End()
	x.log.InfoContext(ctx, "local timeout")

	// has the validator voted in this round, if true send the same (timeout)vote
	// maybe less than quorum of nodes where operational the last time
	timeoutVoteMsg := x.pacemaker.GetTimeoutVote()
	if timeoutVoteMsg == nil {
		// create timeout vote
		timeoutVoteMsg = abdrc.NewTimeoutMsg(
			drctypes.NewTimeout(x.pacemaker.GetCurrentRound(), 0, x.blockStore.GetHighQc()),
			x.id.String(),
			x.pacemaker.LastRoundTC())
		if err := x.safety.SignTimeout(timeoutVoteMsg, x.pacemaker.LastRoundTC()); err != nil {
			x.log.WarnContext(ctx, "failed to sign timeout", logger.Error(err))
			return
		}
		if err := x.blockStore.StoreLastVote(timeoutVoteMsg); err != nil {
			x.log.WarnContext(ctx, "failed to store timeout vote", logger.Error(err))
		}
		x.pacemaker.SetTimeoutVote(timeoutVoteMsg)
	}
	// in the case root chain has not made any progress (less than quorum nodes online), broadcast the same vote again
	// broadcast timeout vote
	x.log.LogAttrs(ctx, logger.LevelTrace, "broadcasting timeout vote")
	if err := x.net.Send(ctx, timeoutVoteMsg, x.leaderSelector.GetNodes()...); err != nil {
		x.log.WarnContext(ctx, "error on broadcasting timeout vote", logger.Error(err))
	}
	x.voteCnt.Add(ctx, 1, attrSetVoteForTC)
}

// onPartitionIRChangeReq handle partition change requests. Received from go routine handling
// partition communication when either partition reaches consensus or cannot reach consensus.
func (x *ConsensusManager) onPartitionIRChangeReq(ctx context.Context, req *IRChangeRequest) (rErr error) {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onPartitionIRChangeReq", trace.WithAttributes(observability.Round(x.pacemaker.GetCurrentRound())))
	defer func() {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		span.End()
	}()

	irReq := &drctypes.IRChangeReq{
		Partition: req.Partition,
		Shard:     req.Shard,
		Requests:  req.Requests,
	}
	switch req.Reason {
	case Quorum:
		irReq.CertReason = drctypes.Quorum
	case QuorumNotPossible:
		irReq.CertReason = drctypes.QuorumNotPossible
	default:
		return fmt.Errorf("invalid IR change request from partition %s: unknown reason %v", irReq.Partition, req.Reason)
	}

	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	if nextLeader == x.id {
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irReq, x.irReqVerifier); err != nil {
			return fmt.Errorf("failed to add IR change request from partition %s into buffer: %w", irReq.Partition, err)
		}
		x.log.DebugContext(ctx, "IR change request buffered", logger.Shard(req.Partition, req.Shard))
		return nil
	}
	// forward to leader
	irMsg := &abdrc.IrChangeReqMsg{
		Author:      x.id.String(),
		IrChangeReq: irReq,
	}
	if err := x.safety.Sign(irMsg); err != nil {
		return fmt.Errorf("failed to sign IR change request from partition %s: %w", irReq.Partition, err)
	}
	if err := x.net.Send(ctx, irMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to send IR change request from partition %s: %w", irReq.Partition, err)
	}
	x.log.DebugContext(ctx, fmt.Sprintf("IR change request forwarded to %s", nextLeader.ShortString()), logger.Shard(req.Partition, req.Shard))
	return nil
}

// onIRChangeMsg handles IR change request messages from other root nodes
func (x *ConsensusManager) onIRChangeMsg(ctx context.Context, irChangeMsg *abdrc.IrChangeReqMsg) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onIRChangeMsg")
	defer span.End()

	if err := irChangeMsg.Verify(x.trustBase); err != nil {
		return fmt.Errorf("invalid IR change request from node %s: %w", irChangeMsg.Author, err)
	}
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	// if the node will be the next leader then buffer the request to be included in the block proposal
	// todo: if in recovery then forward to next?
	if nextLeader == x.id {
		if err := x.irReqBuffer.Add(x.pacemaker.GetCurrentRound(), irChangeMsg.IrChangeReq, x.irReqVerifier); err != nil {
			// if duplicate - the same is already in progress, then it is ok; this is just most likely a delayed request
			if errors.Is(err, ErrDuplicateChangeReq) {
				return nil
			}
			return fmt.Errorf("failed to add IR change request from %s into buffer: %w", irChangeMsg.Author, err)
		}
		x.log.DebugContext(ctx, fmt.Sprintf("IR change request from node %s buffered", irChangeMsg.Author), logger.Shard(irChangeMsg.IrChangeReq.Partition, irChangeMsg.IrChangeReq.Shard))
		return nil
	}
	// todo: AB-549 add max hop count or some sort of TTL?
	// either this is a completely lost message or because of race we just proposed, forward the original
	// message again to next leader
	x.fwdIRCRCnt.Add(ctx, 1, observability.Shard(irChangeMsg.IrChangeReq.Partition, irChangeMsg.IrChangeReq.Shard, attribute.String("reason", irChangeMsg.IrChangeReq.CertReason.String())))
	if err := x.net.Send(ctx, irChangeMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to forward IR change request from %s to the next leader: %w", irChangeMsg.Author, err)
	}
	return nil
}

// onVoteMsg handle votes messages from other root validators
func (x *ConsensusManager) onVoteMsg(ctx context.Context, vote *abdrc.VoteMsg) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onVoteMsg", trace.WithAttributes(observability.Round(vote.VoteInfo.RoundNumber)))
	defer span.End()

	if vote.VoteInfo.RoundNumber < x.pacemaker.GetCurrentRound() {
		x.susVotes.Add(ctx, 1, attrSetQCVoteStale)
		return fmt.Errorf("stale vote for round %d from %s", vote.VoteInfo.RoundNumber, vote.Author)
	}
	// verify signature on vote
	if err := vote.Verify(x.trustBase); err != nil {
		return fmt.Errorf("invalid vote: %w", err)
	}
	// if a vote is received for future round it is intended for the node which is going to be the
	// leader. Cache the vote and wait for more one vote is not enough to trigger recovery.
	// If the node has received at least f+1 votes, then at least 1 honest node also agrees that this node
	// should be the next leader - try and recover.
	if vote.VoteInfo.RoundNumber > x.pacemaker.GetCurrentRound() {
		// either vote arrived before proposal or node is just behind (others think we're the leader of
		// the round, but we haven't seen QC or TC for previous round?)
		// Votes are buffered for one round only so if we overwrite author's vote it is either stale
		// or we have received the vote more than once.
		x.susVotes.Add(ctx, 1, attrSetQCVoteEarly)
		x.log.DebugContext(ctx, "received vote for future round")
		x.voteBuffer[vote.Author] = vote
		// if we have received quorum votes, but no proposal yet or otherwise behind, then try and recover.
		// other nodes seem to think we are the next leader
		// NB! it seems that it's quite common that votes arrive before proposal and going into recovery
		// too early is counterproductive... maybe do not trigger recovery here at all - if we're lucky
		// proposal will arrive on time, otherwise round will likely TO anyway?
		if uint64(len(x.voteBuffer)) >= x.trustBase.GetQuorumThreshold() {
			err := fmt.Errorf("have received %d votes but no proposal, entering recovery", len(x.voteBuffer))
			if e := x.sendRecoveryRequests(ctx, vote); e != nil {
				err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
			}
			return err
		}
		return nil
	}
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

	// Normal votes are only sent to the next leader (timeout votes are broadcast) is it us?
	// NB! we assume vote.VoteInfo.RoundNumber == x.pacemaker.GetCurrentRound() but it also could be that VVR > CR+1
	nextRound := vote.VoteInfo.RoundNumber + 1
	if x.leaderSelector.GetLeaderForRound(nextRound) != x.id {
		return fmt.Errorf("validator is not the leader for round %d", nextRound)
	}

	qc, mature, err := x.pacemaker.RegisterVote(vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register vote: %w", err)
	}
	x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("processed vote, quorum: %t, mature: %t", qc != nil, mature))
	if qc != nil && mature {
		x.processQC(ctx, qc)
		x.processNewRoundEvent(ctx)
		x.updateQCMetrics(ctx, qc)
	}
	return nil
}

// onTimeoutMsg handles timeout vote messages from other root validators
// Timeout votes are broadcast to all nodes on local timeout and all validators try to assemble
// timeout certificate independently.
func (x *ConsensusManager) onTimeoutMsg(ctx context.Context, vote *abdrc.TimeoutMsg) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onTimeoutMsg")
	defer span.End()
	if vote.Timeout.Round < x.pacemaker.GetCurrentRound() {
		return fmt.Errorf("stale timeout vote for round %d from %s", vote.Timeout.Round, vote.Author)
	}
	// verify signature on vote
	if err := vote.Verify(x.trustBase); err != nil {
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
	x.processTC(ctx, vote.LastTC)

	tc, err := x.pacemaker.RegisterTimeoutVote(ctx, vote, x.trustBase)
	if err != nil {
		return fmt.Errorf("failed to register timeout vote: %w", err)
	}
	if tc == nil {
		x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("processed timeout vote for round %d, no quorum yet", vote.Timeout.Round))
		return nil
	}
	x.log.DebugContext(ctx, fmt.Sprintf("timeout quorum for round %d achieved", vote.Timeout.Round))
	// process timeout certificate to advance to next the view/round
	x.processTC(ctx, tc)
	// if this node is the leader in this round then issue a proposal
	l := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound())
	if l == x.id {
		x.processNewRoundEvent(ctx)
	} else {
		x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("new leader is %s, waiting for proposal", l.String()))
	}
	return nil
}

/*
checkRecoveryNeeded verify current state against received state and determine if the validator needs to
recover or not. Basically either the state is different or validator is behind (has skipped some views/rounds).
Returns nil when no recovery is needed and error describing the reason to trigger the recovery otherwise.
*/
func (x *ConsensusManager) checkRecoveryNeeded(qc *drctypes.QuorumCert) error {
	// when node has fallen behind we trigger recovery here as we fail to load block for
	// the round - it would be nicer to have explicit round check?
	block, err := x.blockStore.Block(qc.VoteInfo.RoundNumber)
	if err != nil {
		return fmt.Errorf("failed to read root hash for round %d from local block store: %w", qc.VoteInfo.RoundNumber, err)
	}
	if !bytes.Equal(qc.VoteInfo.CurrentRootHash, block.RootHash) {
		return fmt.Errorf("unexpected round %d state - expected %X, local %X", qc.VoteInfo.RoundNumber, qc.VoteInfo.CurrentRootHash, block.RootHash)
	}
	return nil
}

// onProposalMsg handles block proposal messages from other validators.
// Only a proposal made by the leader of this view/round shall be accepted and processed
func (x *ConsensusManager) onProposalMsg(ctx context.Context, proposal *abdrc.ProposalMsg) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onProposalMsg")
	defer span.End()
	if proposal.Block.Round < x.pacemaker.GetCurrentRound() {
		return fmt.Errorf("stale proposal for round %d from %s", proposal.Block.Round, proposal.Block.Author)
	}
	// verify signature on proposal (does not verify partition request signatures)
	if err := proposal.Verify(x.trustBase); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	// Check current state against new QC
	if err := x.checkRecoveryNeeded(proposal.Block.Qc); err != nil {
		err = fmt.Errorf("proposal triggers recovery: %w", err)
		if e := x.sendRecoveryRequests(ctx, proposal); e != nil {
			err = errors.Join(err, fmt.Errorf("sending recovery requests failed: %w", e))
		}
		return err
	}
	// Is from valid leader
	if l := x.leaderSelector.GetLeaderForRound(proposal.Block.Round).String(); l != proposal.Block.Author {
		return fmt.Errorf("expected %s to be leader of the round %d but got proposal from %s", l, proposal.Block.Round, proposal.Block.Author)
	}
	// Every proposal must carry a QC or TC for previous round
	// Process QC first, update round
	x.processQC(ctx, proposal.Block.Qc)
	x.processTC(ctx, proposal.LastRoundTc)
	// execute proposed payload
	start := time.Now()
	execStateId, err := x.blockStore.Add(proposal.Block, x.irReqVerifier)
	x.addBlockDur.Record(ctx, time.Since(start).Seconds())
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
	if err = x.blockStore.StoreLastVote(voteMsg); err != nil {
		x.log.WarnContext(ctx, "vote store failed", logger.Error(err))
	}
	x.pacemaker.SetVoted(voteMsg)
	// send vote to the next leader
	nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
	x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("sending vote to next leader %s, round %d", nextLeader.String(), proposal.Block.Round))
	x.voteCnt.Add(ctx, 1, attrSetVoteForQC)
	if err = x.net.Send(ctx, voteMsg, nextLeader); err != nil {
		return fmt.Errorf("failed to send vote to next leader: %w", err)
	}
	x.replayVoteBuffer(ctx)
	// if everything goes fine, but the node is in recovery state, then clear it
	// most likely ended up here because proposal was late and votes arrived before
	if x.recovery.Clear() != nil {
		x.log.DebugContext(ctx, "clearing recovery state on proposal")
	}
	return nil
}

// processQC - handles quorum certificate
func (x *ConsensusManager) processQC(ctx context.Context, qc *drctypes.QuorumCert) {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.processQC")
	defer span.End()
	if qc == nil {
		return
	}
	certs, err := x.blockStore.ProcessQc(qc)
	if err != nil {
		x.log.WarnContext(ctx, "failure to process QC triggers recovery", logger.Error(err))
		if err := x.sendRecoveryRequests(ctx, qc); err != nil {
			x.log.WarnContext(ctx, "sending recovery requests failed", logger.Error(err))
		}
		return
	}
	select {
	case <-ctx.Done():
		return // node is exiting certificates have been stored and we are done
	case x.ucSink <- certs: // trigger update to partition nodes
	}

	if !x.pacemaker.AdvanceRoundQC(ctx, qc) {
		return
	}

	// in the "DiemBFT v4" pseudocode the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err := x.leaderSelector.Update(qc, x.pacemaker.GetCurrentRound(), x.blockStore.Block); err != nil {
		x.log.ErrorContext(ctx, "failed to update leader selector", logger.Error(err))
	}
}

// processTC - handles timeout certificate
func (x *ConsensusManager) processTC(ctx context.Context, tc *drctypes.TimeoutCert) {
	_, span := x.tracer.Start(ctx, "ConsensusManager.processTC")
	defer span.End()
	if tc == nil {
		return
	}
	if err := x.blockStore.ProcessTc(tc); err != nil {
		// method deletes the block that got TC - it will never be part of the chain.
		// however, this node might not have even seen the block, in which case error is returned, but this is ok - just log
		x.log.DebugContext(ctx, "could not remove the timeout block, node has not received it", logger.Error(err))
	}
	x.pacemaker.AdvanceRoundTC(ctx, tc)
}

/*
sendCertificates reads UCs produced by processing QC and makes them available for
validator via certResultCh chan (returned by CertificationResult method).
The idea is not to block CM until validator consumes the certificates, ie to
send the UCs in an async fashion.
*/
func (x *ConsensusManager) sendCertificates(ctx context.Context) error {
	// pending certificates, to be consumed by the validator.
	// access to it is "serialized" ie we either update it with
	// new certs sent by CM or we feed it's content to validator
	certs := make(map[types.PartitionShardID]*certification.CertificationResponse)

	feedValidator := func(ctx context.Context) chan struct{} {
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			for key, uc := range certs {
				select {
				case x.certResultCh <- uc:
					delete(certs, key)
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
			for _, uc := range nm {
				certs[types.PartitionShardID{PartitionID: uc.Partition, ShardID: uc.Shard.Key()}] = uc
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
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.replayVoteBuffer")
	defer span.End()

	voteCnt := len(x.voteBuffer)
	if voteCnt == 0 {
		return
	}

	x.log.DebugContext(ctx, fmt.Sprintf("replaying %d buffered votes", voteCnt))
	var errs []error
	for _, v := range x.voteBuffer {
		if err := x.onVoteMsg(ctx, v); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		// log the error(s) rather than return them as failing to process buffered
		// votes is not critical from the callers POV, but we want to have this info
		// for debugging
		err := errors.Join(errs...)
		x.log.WarnContext(ctx, fmt.Sprintf("out of %d buffered votes %d caused error on replay", voteCnt, len(errs)), logger.Error(err))
		span.RecordError(err)
	}

	clear(x.voteBuffer)
}

// processNewRoundEvent handled new view event, is called when either QC or TC is reached locally and
// triggers a new round. If this node is the leader in the new view/round, then make a proposal otherwise
// wait for a proposal from a leader in this round/view
func (x *ConsensusManager) processNewRoundEvent(ctx context.Context) {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.processNewRoundEvent")
	defer span.End()
	round := x.pacemaker.GetCurrentRound()
	if l := x.leaderSelector.GetLeaderForRound(round); l != x.id {
		x.log.InfoContext(ctx, fmt.Sprintf("new round start, not leader, awaiting proposal from %s", l.ShortString()))
		return
	}

	x.leaderCnt.Add(ctx, 1)
	x.log.InfoContext(ctx, "new round start, node is leader")

	// find shards with T2 timeouts
	timedOutShards, err := x.t2Timeouts.GetT2Timeouts(round)
	if err != nil {
		// error here is not fatal, still make a proposal, hopefully the next node will generate timeout
		// requests for partitions this node failed to query
		x.log.WarnContext(ctx, "failed to check timeouts for some partitions", logger.Error(err))
	}
	proposalMsg := &abdrc.ProposalMsg{
		Block: &drctypes.BlockData{
			Version:   1,
			Author:    x.id.String(),
			Round:     round,
			Epoch:     0,
			Timestamp: types.NewTimestamp(),
			Payload:   x.irReqBuffer.GeneratePayload(round, timedOutShards, x.blockStore.IsChangeInProgress),
			Qc:        x.blockStore.GetHighQc(),
		},
		LastRoundTc: x.pacemaker.LastRoundTC(),
	}
	// safety makes simple sanity checks and signs if everything is ok
	if err = x.safety.Sign(proposalMsg); err != nil {
		x.log.WarnContext(ctx, "failed to send proposal message, signing failed", logger.Error(err))
		return
	}
	// broadcast proposal message (also to self)
	span.AddEvent(proposalMsg.Block.String())
	x.log.LogAttrs(ctx, slog.LevelDebug, "broadcast proposal", logger.Data(proposalMsg.Block.String()))
	if err = x.net.Send(ctx, proposalMsg, x.leaderSelector.GetNodes()...); err != nil {
		x.log.WarnContext(ctx, "error on broadcasting proposal message", logger.Error(err))
	}
	for _, cr := range proposalMsg.Block.Payload.Requests {
		x.proposedCR.Add(ctx, 1, observability.Shard(cr.Partition, cr.Shard, attribute.String("reason", cr.CertReason.String())))
	}
}

func (x *ConsensusManager) onStateReq(ctx context.Context, req *abdrc.StateRequestMsg) error {
	if x.recovery.InRecovery() {
		return fmt.Errorf("node is in recovery: %s", x.recovery)
	}
	peerID, err := peer.Decode(req.NodeId)
	if err != nil {
		return fmt.Errorf("invalid receiver identifier %q: %w", req.NodeId, err)
	}
	stateMsg, err := x.blockStore.GetState()
	if err != nil {
		return fmt.Errorf("creating state message: %w", err)
	}
	if err = x.net.Send(ctx, stateMsg, peerID); err != nil {
		return fmt.Errorf("failed to send state response message: %w", err)
	}
	return nil
}

func (x *ConsensusManager) onStateResponse(ctx context.Context, rsp *abdrc.StateMsg) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.onStateResponse")
	defer span.End()
	x.log.LogAttrs(ctx, logger.LevelTrace, "received state response; recoveryState: "+x.recovery.String())
	if !x.recovery.InRecovery() {
		// we do send out multiple state recovery request so do not return error when we ignore the ones after successful recovery...
		return nil
	}
	if err := rsp.Verify(x.params.HashAlgorithm, x.trustBase); err != nil {
		return fmt.Errorf("recovery response verification failed: %w", err)
	}
	if err := rsp.CanRecoverToRound(x.recovery.ToRound()); err != nil {
		return fmt.Errorf("state message not suitable for recovery: %w", err)
	}
	// sort blocks by round
	slices.SortFunc(rsp.Pending, func(a, b *drctypes.BlockData) int {
		return cmp.Compare(a.GetRound(), b.GetRound())
	})
	blockStore, err := storage.NewFromState(x.params.HashAlgorithm, rsp.CommittedHead, x.blockStore.GetDB(), x.orchestration, x.log)
	if err != nil {
		return fmt.Errorf("recovery, new block store init failed: %w", err)
	}
	// create new verifier
	reqVerifier, err := NewIRChangeReqVerifier(x.params, blockStore)
	if err != nil {
		return fmt.Errorf("verifier construction failed: %w", err)
	}
	x.pacemaker.Reset(ctx, blockStore.GetHighQc().GetRound(), nil, nil)

	for i, block := range rsp.Pending {
		// if received block has QC then process it first as with a block received normally
		if block.Qc != nil {
			if _, err = blockStore.ProcessQc(block.Qc); err != nil {
				if i != 0 || !errors.Is(err, storage.ErrCommitFailed) {
					// since history is only kept until the last committed round it is not possible to commit a previous round
					return fmt.Errorf("block %d for round %v add qc failed: %w", i, block.GetRound(), err)
				}
				x.log.DebugContext(ctx, "processing QC from recovery block", logger.Error(err))
			}
			x.pacemaker.AdvanceRoundQC(ctx, block.Qc)
		}
		if _, err = blockStore.Add(block, reqVerifier); err != nil {
			return fmt.Errorf("failed to add recovery block %d: %w", i, err)
		}
	}
	t2TimeoutGen, err := NewLucBasedT2TimeoutGenerator(x.params, blockStore)
	if err != nil {
		return fmt.Errorf("recovery T2 timeout generator init failed: %w", err)
	}
	// all ok
	x.blockStore = blockStore
	x.irReqVerifier = reqVerifier
	x.t2Timeouts = t2TimeoutGen
	// exit recovery status and replay buffered messages
	x.log.DebugContext(ctx, "completed recovery")
	triggerMsg := x.recovery.Clear()
	// in the "DiemBFT v4" pseudocode the process_certificate_qc first calls
	// leaderSelector.Update and after that pacemaker.AdvanceRound - we do it the
	// other way around as otherwise current leader goes out of sync with peers...
	if err = x.leaderSelector.Update(x.blockStore.GetHighQc(), x.pacemaker.GetCurrentRound(), x.blockStore.Block); err != nil {
		x.log.ErrorContext(ctx, "failed to update leader selector", logger.Error(err))
	}
	if prop, ok := triggerMsg.(*abdrc.ProposalMsg); ok {
		// the proposal was verified when it was received, so try and execute it now
		// Every proposal must carry a QC or TC for previous round
		// Process QC first, update round
		x.processQC(ctx, prop.Block.Qc)
		x.processTC(ctx, prop.LastRoundTc)
		var stateHash []byte
		if block, err := x.blockStore.Block(prop.Block.Round); err != nil {
			// Block not found, was not sent with recovery info
			// execute proposed payload
			stateHash, err = x.blockStore.Add(prop.Block, x.irReqVerifier)
			if err != nil {
				// wait for timeout, if others make progress this node will need to recover
				// cannot send vote, so just return and wait for local timeout or new proposal (and try to recover then)
				return fmt.Errorf("recovery failed to execute proposal: %w", err)
			}
		} else {
			// use the state hash from storage
			stateHash = block.RootHash
		}
		// send a vote message to next leader
		voteMsg, err := x.safety.MakeVote(prop.Block, stateHash, x.blockStore.GetHighQc(), x.pacemaker.LastRoundTC())
		if err != nil {
			// wait for timeout, if others make progress this node will need to recover
			return fmt.Errorf("failed to sign vote: %w", err)
		}
		x.pacemaker.SetVoted(voteMsg)
		// send vote to the next leader
		nextLeader := x.leaderSelector.GetLeaderForRound(x.pacemaker.GetCurrentRound() + 1)
		x.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("sending block %d vote after recovery to next leader %s", prop.Block.Round, nextLeader.String()))
		if err = x.net.Send(ctx, voteMsg, nextLeader); err != nil {
			return fmt.Errorf("failed to send vote to next leader: %w", err)
		}
	}
	if tmo, ok := triggerMsg.(*abdrc.TimeoutMsg); ok {
		// timeout vote carries last round TC, if not nil, use it to advance pacemaker to correct round
		// todo: timeout votes are not buffered
		x.processTC(ctx, tmo.LastTC)
	}
	x.replayVoteBuffer(ctx)
	return nil
}

func (x *ConsensusManager) sendRecoveryRequests(ctx context.Context, triggerMsg any) error {
	ctx, span := x.tracer.Start(ctx, "ConsensusManager.sendRecoveryRequests")
	defer span.End()

	signatures, err := x.recovery.Set(triggerMsg)
	if err != nil {
		return err
	}

	x.recoveryReq.Add(ctx, 1)

	if err = x.net.Send(ctx, &abdrc.StateRequestMsg{NodeId: x.id.String()},
		selectRandomNodeIdsFromSignatureMap(signatures, 2)...); err != nil {
		return fmt.Errorf("failed to send recovery request: %w", err)
	}
	return nil
}

/*
selectRandomNodeIdsFromSignatureMap returns slice with up to "count" random keys
from "m" without duplicates. The "count" assumed to be greater than zero, iow the
function always returns at least one item (given that map is not empty).
The key of the "m" must be of type peer.ID encoded as string (if it's not it is ignored).
When "m" has fewer items than "count" then len(m) items is returned (iow all map keys),
when "m" is empty then empty/nil slice is returned.
*/
func selectRandomNodeIdsFromSignatureMap(m map[string]hex.Bytes, count int) (nodes []peer.ID) {
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

func (x *ConsensusManager) ShardInfo(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
	if x.recovery.InRecovery() {
		return nil, fmt.Errorf("node is in recovery: %s", x.recovery)
	}
	si := x.blockStore.ShardInfo(partition, shard)
	if si == nil {
		return nil, fmt.Errorf("unknown partition %s shard %s", partition, shard.String())
	}
	return si, nil
}

/*
updateQCMetrics updates metrics about QC. Only leader should call it, so we get "per round" counts.
*/
func (x *ConsensusManager) updateQCMetrics(ctx context.Context, qc *drctypes.QuorumCert) {
	if qc == nil {
		return
	}

	x.qcSize.Add(ctx, int64(len(qc.Signatures)))

	// when node count gets big this is potentially bad metric (cardinality of the node id)
	// but have it for debugging for now?
	for nodeID := range qc.Signatures {
		x.qcVoters.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.String("node.id", nodeID))))
	}
}

func (x *ConsensusManager) GetState() (*abdrc.StateMsg, error) {
	return x.blockStore.GetState()
}

// "constant" (ie without variable part) attribute sets for observability
var (
	attrSetQCVoteStale = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "stale")))
	attrSetQCVoteEarly = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "early")))

	attrSetVoteForQC = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "proposal")))
	attrSetVoteForTC = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "timeout")))
)
