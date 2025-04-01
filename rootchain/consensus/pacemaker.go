package consensus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

/*
Pacemaker tracks the current round/view number - a new round/view starts if there is a quorum
certificate or timeout certificate for the previous round.
It also provides "round clock" which allows to make sure that rounds are not produced too fast
but also do not take too long (timeout).
In addition it keeps track of validator data related to the active round (votes received if
next leader or votes sent if follower).
*/
type Pacemaker struct {
	// minimum round duration, ie rounds shouldn't advance faster than that
	minRoundLen time.Duration
	// max round duration, after that round is considered to be timed out
	maxRoundLen time.Duration
	// Last commit.
	lastQcToCommitRound uint64
	// Current round is max(highest QC, highest TC) + 1.
	currentRound atomic.Uint64
	// Collection of votes (when node is the next leader)
	pendingVotes *VoteRegister
	// Last round timeout certificate
	lastRoundTC *types.TimeoutCert
	// Store for votes sent in the ongoing round.
	voteSent    *abdrc.VoteMsg
	timeoutVote *abdrc.TimeoutMsg
	currentQC   *types.QuorumCert

	statusUpd      sync.Mutex // lock to sync status (status and statusChan) update
	status         atomic.Uint32
	statusChan     chan paceMakerStatus
	stopRoundClock context.CancelFunc

	tracer   trace.Tracer
	roundDur metric.Float64Histogram
	roundCnt metric.Int64Counter
}

/*
NewPacemaker initializes new Pacemaker instance (zero value is not usable).

  - minRoundLen is the minimum round duration, rounds shouldn't advance faster than that;
  - maxRoundLen is maximum round duration, after that round is considered to be timed out;

The maxRoundLen must be greater than minRoundLen or the Pacemaker will crash at some point!
*/
func NewPacemaker(minRoundLen, maxRoundLen time.Duration, observe Observability) (*Pacemaker, error) {
	pm := &Pacemaker{
		minRoundLen:    minRoundLen,
		maxRoundLen:    maxRoundLen,
		pendingVotes:   NewVoteRegister(),
		statusChan:     make(chan paceMakerStatus, 1),
		stopRoundClock: func() { /* init as NOP */ },
		tracer:         observe.Tracer("pacemaker"),
	}

	var err error
	m := observe.Meter("pacemaker")
	// we expect that the minRoundLen is relatively short (~0.25s) while maxRoundLen is
	// relatively long (~10s). We hope that most rounds last only little bit longer than
	// minRoundLen so generate few buckets between min and max with finer steps near the min end.
	step := (25 * time.Millisecond).Seconds()
	buckets := []float64{minRoundLen.Seconds()}
	for i := 0; buckets[i] < 2*maxRoundLen.Seconds(); i++ {
		n := time.Duration((buckets[i] + step) * float64(time.Second)).Truncate(time.Millisecond)
		buckets = append(buckets, n.Seconds())
		step *= 2
	}
	pm.roundDur, err = m.Float64Histogram("round.duration",
		metric.WithDescription("How long it took from the start of round R to the start of R+1"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(buckets...))
	if err != nil {
		return nil, fmt.Errorf("creating histogram for round duration: %w", err)
	}
	pm.roundCnt, err = m.Int64Counter("round", metric.WithDescription("How many new rounds have been started"))
	if err != nil {
		return nil, fmt.Errorf("creating round counter: %w", err)
	}

	return pm, nil
}

/*
Reset sets the pacemaker's "last committed round" and starts next round.
This method should only used to start the pacemaker and reset it's status
on system recovery, during normal operation current round is advanced by
calling AdvanceRoundQC or AdvanceRoundTC.
*/
func (x *Pacemaker) Reset(ctx context.Context, highQCRound uint64, lastTc *types.TimeoutCert, lastVote any) {
	ctx, span := x.tracer.Start(ctx, "Pacemaker.Reset")
	defer span.End()

	x.lastRoundTC = nil
	x.lastQcToCommitRound = highQCRound
	lastRound := highQCRound
	// If TC is from a more recent round then use it instead
	if highQCRound < lastTc.GetRound() {
		lastRound = lastTc.GetRound()
		x.lastRoundTC = lastTc
	}
	x.startNewRound(ctx, lastRound+1)
	// restore last sent vote for pace maker
	switch vote := lastVote.(type) {
	case *abdrc.VoteMsg:
		x.SetVoted(vote)
	case *abdrc.TimeoutMsg:
		x.SetTimeoutVote(vote)
	}
}

func (x *Pacemaker) Stop() {
	x.stopRoundClock()
}

func (x *Pacemaker) LastRoundTC() *types.TimeoutCert {
	return x.lastRoundTC
}

func (x *Pacemaker) GetCurrentRound() uint64 {
	return x.currentRound.Load()
}

// SetVoted - remember vote sent in this view
func (x *Pacemaker) SetVoted(vote *abdrc.VoteMsg) {
	if vote.VoteInfo.RoundNumber == x.currentRound.Load() {
		x.voteSent = vote
	}
}

// GetVoted - has the node voted in this round, returns either vote or nil
func (x *Pacemaker) GetVoted() *abdrc.VoteMsg {
	return x.voteSent
}

// SetTimeoutVote - remember timeout vote sent in this view
func (x *Pacemaker) SetTimeoutVote(vote *abdrc.TimeoutMsg) {
	if vote.Timeout.Round == x.currentRound.Load() {
		x.timeoutVote = vote
	}
}

// GetTimeoutVote - has the node voted for timeout in this round, returns either vote or nil
func (x *Pacemaker) GetTimeoutVote() *abdrc.TimeoutMsg {
	return x.timeoutVote
}

/*
RoundQC returns the latest QC produced by calling RegisterVote.
*/
func (x *Pacemaker) RoundQC() *types.QuorumCert {
	return x.currentQC
}

/*
RegisterVote register vote for the round and assembles quorum certificate when quorum condition is met.
It returns non nil QC in case of quorum is achieved.
It also returns bool which indicates is the round "mature", ie it has lasted at least the minimum
required amount of time to make proposal.
*/
func (x *Pacemaker) RegisterVote(vote *abdrc.VoteMsg, quorum QuorumInfo) (*types.QuorumCert, bool, error) {
	// If the vote is not about the current round then ignore
	if round := x.currentRound.Load(); vote.VoteInfo.RoundNumber != round {
		return nil, false, fmt.Errorf("received vote is for round %d, current round is %d", vote.VoteInfo.RoundNumber, round)
	}
	qc, err := x.pendingVotes.InsertVote(vote, quorum)
	if err != nil {
		return nil, false, fmt.Errorf("vote register error: %w", err)
	}
	x.currentQC = qc
	return qc, x.roundIsMature(), nil
}

/*
RegisterTimeoutVote registers time-out vote from root node (including vote from self) and tries to assemble
a timeout quorum certificate for the round.
*/
func (x *Pacemaker) RegisterTimeoutVote(ctx context.Context, vote *abdrc.TimeoutMsg, quorum QuorumInfo) (*types.TimeoutCert, error) {
	tc, voteCnt, err := x.pendingVotes.InsertTimeoutVote(vote, quorum)
	if err != nil {
		return nil, fmt.Errorf("inserting to pending votes: %w", err)
	}
	if tc == nil && voteCnt > quorum.GetMaxFaultyNodes() && x.status.Load() != uint32(pmsRoundTimeout) {
		// there is f+1 votes for TO - jump to TO state as quorum shouldn't be possible now
		x.setState(ctx, pmsRoundTimeout)
	}
	return tc, nil
}

// AdvanceRoundQC - trigger next round/view on quorum certificate
func (x *Pacemaker) AdvanceRoundQC(ctx context.Context, qc *types.QuorumCert) bool {
	ctx, span := x.tracer.Start(ctx, "Pacemaker.AdvanceRoundQC")
	defer span.End()

	// Same QC can only advance the round number once
	if qc == nil || qc.VoteInfo.RoundNumber < x.currentRound.Load() {
		return false
	}

	x.lastRoundTC = nil
	// only increment high committed round if QC commits a state
	if qc.LedgerCommitInfo.Hash != nil {
		x.lastQcToCommitRound = qc.VoteInfo.RoundNumber
	}
	x.startNewRound(ctx, qc.VoteInfo.RoundNumber+1)
	x.roundCnt.Add(ctx, 1, attrSetNextRoundQC)
	return true
}

// AdvanceRoundTC - trigger next round/view on timeout certificate
func (x *Pacemaker) AdvanceRoundTC(ctx context.Context, tc *types.TimeoutCert) {
	ctx, span := x.tracer.Start(ctx, "Pacemaker.AdvanceRoundTC")
	defer span.End()

	// no timeout cert or is from old view/round - ignore
	if tc == nil || tc.Timeout.Round < x.currentRound.Load() {
		return
	}

	x.lastRoundTC = tc
	x.startNewRound(ctx, tc.Timeout.Round+1)
	x.roundCnt.Add(ctx, 1, attrSetNextRoundTC)
}

/*
startNewRound - sets new current round number, resets all stores and
starts round clock which produces events into StatusEvents channel.
*/
func (x *Pacemaker) startNewRound(ctx context.Context, round uint64) {
	ctx, span := x.tracer.Start(ctx, "Pacemaker.startNewRound", trace.WithAttributes(observability.Round(round)))
	defer span.End()

	x.stopRoundClock()
	start := time.Now()
	x.currentQC = nil
	x.voteSent = nil
	x.timeoutVote = nil
	x.pendingVotes.Reset()
	x.currentRound.Store(round)

	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopped := x.startRoundClock(ctx, x.minRoundLen, x.maxRoundLen)
	x.stopRoundClock = func() {
		cancel()
		<-stopped
		x.roundDur.Record(context.Background(), time.Since(start).Seconds())
	}
}

/*
startRoundClock manages round state and generates appropriate events into chan returned by StatusEvents.
Round state normally changes pmsRoundInProgress -> pmsRoundMatured -> pmsRoundTimeout. For how long each
state lasts is determined by "minRoundLen" and "maxRoundLen" parameters.
Between stopRoundClock and startRoundClock calls the round status is pmsRoundNone.

It returns chan which is closed when round clock has been stopped (the ctx has been cancelled). When
the clock has been stopped there should be no event in the status event chan and the "status" field
is set to pmsRoundNone.
*/
func (x *Pacemaker) startRoundClock(ctx context.Context, minRoundLen, maxRoundLen time.Duration) <-chan struct{} {
	ctx, span := x.tracer.Start(ctx, "Pacemaker.startRoundClock")
	defer span.End()

	x.status.Store(uint32(pmsRoundInProgress))
	ticker := time.NewTicker(minRoundLen)
	stopped := make(chan struct{})

	go func() {
		ctx, span := x.tracer.Start(ctx, "Pacemaker.roundClock", trace.WithNewRoot(), trace.WithAttributes(observability.Round(x.currentRound.Load())), trace.WithLinks(trace.LinkFromContext(ctx)))
		defer func() {
			close(stopped)
			ticker.Stop()
			span.End()
		}()

		for {
			select {
			case <-ctx.Done():
				select {
				case <-x.statusChan:
				default:
				}
				x.status.Store(uint32(pmsRoundNone))
				return
			case <-ticker.C:
				switch paceMakerStatus(x.status.Load()) {
				case pmsRoundInProgress:
					x.setState(ctx, pmsRoundMatured)
					ticker.Reset(maxRoundLen - minRoundLen)
				case pmsRoundMatured:
					x.setState(ctx, pmsRoundTimeout)
					ticker.Reset(maxRoundLen)
				case pmsRoundTimeout:
					x.setState(ctx, pmsRoundTimeout)
					ticker.Reset(maxRoundLen)
				}
			}
		}
	}()

	return stopped
}

func (x *Pacemaker) setState(ctx context.Context, state paceMakerStatus) {
	_, span := x.tracer.Start(ctx, "Pacemaker.setState", trace.WithAttributes(attribute.Stringer("status", state)))
	defer span.End()

	x.statusUpd.Lock()
	defer x.statusUpd.Unlock()

	select {
	case <-x.statusChan:
	default:
	}

	x.status.Store(uint32(state))
	x.statusChan <- state
}

/*
StatusEvents returns channel into which events are posted when round state changes.

Events are produced once per state change, except pmsRoundTimeout which will be repeated
every time maxRoundLen elapses and new round hasn't been started yet.

pmsRoundInProgress (ie new round started) event is never produced!
*/
func (x *Pacemaker) StatusEvents() <-chan paceMakerStatus { return x.statusChan }

/*
roundIsMature returns true when round is "mature enough" (ie has lasted longer than minRoundLen)
to advance system into next round. Note that timed out round is "ready" too!
*/
func (x *Pacemaker) roundIsMature() bool {
	status := paceMakerStatus(x.status.Load())
	return status == pmsRoundMatured || status == pmsRoundTimeout
}

// pacemaker round statuses
type paceMakerStatus uint32

func (pms paceMakerStatus) String() string {
	switch pms {
	case pmsRoundNone:
		return "pmsRoundNone"
	case pmsRoundInProgress:
		return "pmsRoundInProgress"
	case pmsRoundMatured:
		return "pmsRoundMatured"
	case pmsRoundTimeout:
		return "pmsRoundTimeout"
	}
	return fmt.Sprintf("unknown status %d", uint32(pms))
}

const (
	// no round in progress, ie time between one round ending and starting the next one
	pmsRoundNone paceMakerStatus = 0
	// round clock has been started, need to wait for the minimum round duration to pass
	pmsRoundInProgress paceMakerStatus = 1
	// minimum round duration has elapsed, it's ok to finish the round as soon as quorum is achieved
	pmsRoundMatured paceMakerStatus = 2
	// round has lasted longer than max round duration
	pmsRoundTimeout paceMakerStatus = 3
)

var (
	attrSetNextRoundQC = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "qc")))
	attrSetNextRoundTC = metric.WithAttributeSet(attribute.NewSet(attribute.String("reason", "tc")))
)
