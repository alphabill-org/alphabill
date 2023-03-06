package distributed

import (
	"math"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	TimeoutCalculator interface {
		GetNextTimeout(roundIndexAfterCommit uint64) time.Duration
	}
	// ExponentialTimeInterval exponential back-off
	// baseMs * exponentBase^"commit gap"
	// If max exponent is set to 0, then it will output constant value baseMs
	ExponentialTimeInterval struct {
		baseMs       time.Duration
		exponentBase float64
		maxExponent  uint8
	}

	// Pacemaker tracks the current round/view number. The number is incremented when new quorum are
	// received. A new round/view starts if there is a quorum certificate or timeout certificate for previous round.
	// In addition, it also calculates the local timeout interval based on how many rounds have failed and keeps track
	// of validator data related to the active round (votes received if next leader or votes sent if follower).
	Pacemaker struct {
		blockRate time.Duration
		// Last commit.
		lastQcToCommitRound uint64
		// Current round is max(highest QC, highest TC) + 1.
		currentRound uint64
		// The deadline for the next local timeout event. It is reset every time a new round start, or
		// a previous deadline expires.
		roundTimeout time.Time
		// Time last proposal was received
		lastViewChange time.Time
		// timeout calculator
		timeoutCalculator TimeoutCalculator
		// Collection of votes (when node is the next leader)
		pendingVotes *VoteRegister
		// Last round timeout certificate
		lastRoundTC *atomic_broadcast.TimeoutCert
		// Store for votes sent in the ongoing round.
		voteSent    *atomic_broadcast.VoteMsg
		timeoutVote *atomic_broadcast.TimeoutMsg
	}
)

func (x *Pacemaker) LastRoundTC() *atomic_broadcast.TimeoutCert {
	return x.lastRoundTC
}

func (x ExponentialTimeInterval) GetNextTimeout(roundsAfterLastCommit uint64) time.Duration {
	exp := util.Min(uint64(x.maxExponent), roundsAfterLastCommit)
	mul := math.Pow(x.exponentBase, float64(exp))
	return time.Duration(float64(x.baseMs) * mul)
}

// NewPacemaker Needs to be constructed from last QC!
func NewPacemaker(lastRound uint64, localTimeout time.Duration, bRate time.Duration) *Pacemaker {
	return &Pacemaker{
		blockRate:           bRate,
		lastQcToCommitRound: lastRound,
		currentRound:        lastRound + 1,
		roundTimeout:        time.Now().Add(localTimeout),
		lastViewChange:      time.Now(),
		timeoutCalculator:   ExponentialTimeInterval{baseMs: localTimeout, exponentBase: 1.2, maxExponent: 0},
		pendingVotes:        NewVoteRegister(),
		lastRoundTC:         nil,
		voteSent:            nil,
		timeoutVote:         nil,
	}
}

func (x *Pacemaker) clear() {
	x.voteSent = nil
	x.timeoutVote = nil
}

func (x *Pacemaker) GetCurrentRound() uint64 {
	return x.currentRound
}

func (x *Pacemaker) SetVoted(vote *atomic_broadcast.VoteMsg) {
	if vote.VoteInfo.RoundNumber == x.currentRound {
		x.voteSent = vote
	}
}

func (x *Pacemaker) SetTimeoutVote(vote *atomic_broadcast.TimeoutMsg) {
	if vote.Timeout.Round == x.currentRound {
		x.timeoutVote = vote
	}
}

func (x *Pacemaker) GetVoted() *atomic_broadcast.VoteMsg {
	return x.voteSent
}
func (x *Pacemaker) GetTimeoutVote() *atomic_broadcast.TimeoutMsg {
	return x.timeoutVote
}

func (x *Pacemaker) GetRoundTimeout() time.Duration {
	return x.roundTimeout.Sub(time.Now())
}

func (x *Pacemaker) RegisterVote(vote *atomic_broadcast.VoteMsg, quorum QuorumInfo) *atomic_broadcast.QuorumCert {
	// If the vote is not about the current round then ignore
	if vote.VoteInfo.RoundNumber != x.currentRound {
		logger.Warning("Round %v received vote for unexpected round %v: vote ignored",
			x.currentRound, vote.VoteInfo.RoundNumber)
		return nil
	}
	qc, err := x.pendingVotes.InsertVote(vote, quorum)
	if err != nil {
		logger.Warning("Round %v vote message from %v error:", x.currentRound, vote.Author, err)
		return nil
	}
	return qc
}

func (x *Pacemaker) CalcTimeTilNextProposal(round uint64) time.Duration {
	// according to spec. in case of 2-chain-rule finality the wait is every 2nd block
	now := time.Now()
	if now.Sub(x.lastViewChange) >= time.Duration(round%2)*x.blockRate {
		return 0
	}
	return x.lastViewChange.Add(x.blockRate).Sub(now)
}

func (x *Pacemaker) RegisterTimeoutVote(vote *atomic_broadcast.TimeoutMsg, quorum QuorumInfo) *atomic_broadcast.TimeoutCert {
	tc, err := x.pendingVotes.InsertTimeoutVote(vote, quorum)
	if err != nil {
		logger.Warning("Round %v vote message from %v error:", x.currentRound, vote.Author, err)
		return nil
	}
	return tc
}

func (x *Pacemaker) AdvanceRoundQC(qc *atomic_broadcast.QuorumCert) bool {
	if qc == nil {
		return false
	}
	// Same QC can only advance the round number once
	if qc.VoteInfo.RoundNumber < x.currentRound {
		return false
	}
	x.lastRoundTC = nil
	// only increment high committed round if QC commits a state
	if qc.LedgerCommitInfo.RootHash != nil {
		x.lastQcToCommitRound = qc.VoteInfo.RoundNumber
	}
	x.startNewRound(qc.VoteInfo.RoundNumber + 1)
	return true
}

func (x *Pacemaker) AdvanceRoundTC(tc *atomic_broadcast.TimeoutCert) {
	// no timeout cert or is from old view/round - ignore
	if tc == nil || tc.Timeout.Round < x.currentRound {
		return
	}
	x.lastRoundTC = tc
	x.startNewRound(tc.Timeout.Round + 1)
}

func (x *Pacemaker) startNewRound(round uint64) {
	x.clear()
	x.lastViewChange = time.Now()
	x.currentRound = round
	x.pendingVotes.Reset()
	x.roundTimeout = time.Now().Add(x.timeoutCalculator.GetNextTimeout(x.currentRound - x.lastQcToCommitRound - 1))
}
