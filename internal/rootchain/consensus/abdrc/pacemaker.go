package abdrc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
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
	lastRoundTC *abtypes.TimeoutCert
	// Store for votes sent in the ongoing round.
	voteSent    *abdrc.VoteMsg
	timeoutVote *abdrc.TimeoutMsg
	currentQC   *abtypes.QuorumCert

	statusUpd      sync.Mutex // lock to sync status (status and statusChan) update
	status         atomic.Uint32
	statusChan     chan paceMakerStatus
	ticker         *time.Ticker
	stopRoundClock context.CancelFunc
}

/*
NewPacemaker initializes new Pacemaker instance (zero value is not usable).

  - minRoundLen is the minimum round duration, rounds shouldn't advance faster than that;
  - maxRoundLen is maximum round duration, after that round is considered to be timed out;

The maxRoundLen must be greater than minRoundLen or the Pacemaker will crash at some point!
*/
func NewPacemaker(minRoundLen, maxRoundLen time.Duration) *Pacemaker {
	pm := &Pacemaker{
		minRoundLen:    minRoundLen,
		maxRoundLen:    maxRoundLen,
		pendingVotes:   NewVoteRegister(),
		statusChan:     make(chan paceMakerStatus, 1),
		ticker:         time.NewTicker(maxRoundLen),
		stopRoundClock: func() {},
	}
	pm.ticker.Stop()
	return pm
}

/*
Reset sets the pacemaker's "last committed round" and starts next round.
This method should only used to start the pacemaker and reset it's status
on system recovery, during normal operation current round is advanced by
calling AdvanceRoundQC or AdvanceRoundTC.
*/
func (x *Pacemaker) Reset(highQCRound uint64) {
	x.lastRoundTC = nil
	x.lastQcToCommitRound = highQCRound
	x.startNewRound(highQCRound + 1)
}

func (x *Pacemaker) Stop() {
	x.ticker.Stop()
	x.stopRoundClock()
}

func (x *Pacemaker) LastRoundTC() *abtypes.TimeoutCert {
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
func (x *Pacemaker) RoundQC() *abtypes.QuorumCert {
	return x.currentQC
}

/*
RegisterVote register vote for the round and assembles quorum certificate when quorum condition is met.
It returns non nil QC in case of quorum is achieved.
It also returns bool which indicates is the round "mature", ie it has lasted at least the minimum
required amount of time to make proposal.
*/
func (x *Pacemaker) RegisterVote(vote *abdrc.VoteMsg, quorum QuorumInfo) (*abtypes.QuorumCert, bool, error) {
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
func (x *Pacemaker) RegisterTimeoutVote(vote *abdrc.TimeoutMsg, quorum QuorumInfo) (*abtypes.TimeoutCert, error) {
	tc, voteCnt, err := x.pendingVotes.InsertTimeoutVote(vote, quorum)
	if err != nil {
		return nil, fmt.Errorf("inserting to pending votes: %w", err)
	}
	if tc == nil && voteCnt > quorum.GetMaxFaultyNodes() && x.status.Load() != uint32(pmsRoundTimeout) {
		// there is f+1 votes for TO - jump to TO state as quorum shouldn't be possible now
		x.setState(pmsRoundTimeout, x.maxRoundLen)
	}
	return tc, nil
}

// AdvanceRoundQC - trigger next round/view on quorum certificate
func (x *Pacemaker) AdvanceRoundQC(qc *abtypes.QuorumCert) bool {
	if qc == nil {
		return false
	}
	// Same QC can only advance the round number once
	if qc.VoteInfo.RoundNumber < x.currentRound.Load() {
		return false
	}
	x.lastRoundTC = nil
	// only increment high committed round if QC commits a state
	if qc.LedgerCommitInfo.Hash != nil {
		x.lastQcToCommitRound = qc.VoteInfo.RoundNumber
	}
	x.startNewRound(qc.VoteInfo.RoundNumber + 1)
	return true
}

// AdvanceRoundTC - trigger next round/view on timeout certificate
func (x *Pacemaker) AdvanceRoundTC(tc *abtypes.TimeoutCert) {
	// no timeout cert or is from old view/round - ignore
	if tc == nil || tc.Timeout.Round < x.currentRound.Load() {
		return
	}
	x.lastRoundTC = tc
	x.startNewRound(tc.Timeout.Round + 1)
}

/*
startNewRound - sets new current round number, resets all stores and
starts round clock which produces events into StatusEvents channel.
*/
func (x *Pacemaker) startNewRound(round uint64) {
	x.currentQC = nil
	x.voteSent = nil
	x.timeoutVote = nil
	x.pendingVotes.Reset()
	x.currentRound.Store(round)

	x.stopRoundClock()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := x.startRoundClock(ctx, x.minRoundLen, x.maxRoundLen)
	x.stopRoundClock = func() {
		cancel()
		<-stopped
	}
}

/*
startRoundClock manages round state and generates appropriate events into chan returned by StatusEvents.
Round state normally changes pmsRoundInProgress -> pmsRoundMatured -> pmsRoundTimeout and then stays in
timeout state until next round is started. For how long each state lasts is determined by "minRoundLen"
and "maxRoundLen" parameters.

It returns chan which is closed when round clock has been stopped (the ctx has been cancelled). When
the clock has been stopped there should be no event in the status event chan but the "status" field
still stores the last state of the round.
*/
func (x *Pacemaker) startRoundClock(ctx context.Context, minRoundLen, maxRoundLen time.Duration) <-chan struct{} {
	x.status.Store(uint32(pmsRoundInProgress))
	x.ticker.Reset(minRoundLen)
	select {
	case <-x.ticker.C:
	default:
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-ctx.Done():
				select {
				case <-x.statusChan:
				default:
				}
				return
			case <-x.ticker.C:
				switch paceMakerStatus(x.status.Load()) {
				case pmsRoundInProgress:
					x.setState(pmsRoundMatured, maxRoundLen-minRoundLen)
				case pmsRoundMatured:
					x.setState(pmsRoundTimeout, maxRoundLen)
				case pmsRoundTimeout:
					x.setState(pmsRoundTimeout, maxRoundLen)
				}
			}
		}
	}()

	return stopped
}

func (x *Pacemaker) setState(state paceMakerStatus, tillNextState time.Duration) {
	x.statusUpd.Lock()
	defer x.statusUpd.Unlock()

	select {
	case <-x.statusChan:
	default:
	}

	x.ticker.Reset(tillNextState)
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
func (x *Pacemaker) roundIsMature() bool { return x.status.Load() != uint32(pmsRoundInProgress) }

// pacemaker round statuses
type paceMakerStatus uint32

func (pms paceMakerStatus) String() string {
	switch pms {
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
	// round clock has been started, need to wait for the minimum round duration to pass
	pmsRoundInProgress paceMakerStatus = 1
	// minimum round duration has elapsed, it's ok to finish the round as soon as quorum is achieved
	pmsRoundMatured paceMakerStatus = 2
	// round has lasted longer than max round duration
	pmsRoundTimeout paceMakerStatus = 3
)