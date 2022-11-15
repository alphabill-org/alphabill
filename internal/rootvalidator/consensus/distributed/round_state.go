package distributed

import (
	"math"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
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

	// RoundState tracks the current round/view number. The number is incremented when new quorum are
	// received. A new round/view starts if there is a quorum certificate or timeout certificate for previous round.
	// In addition, it also calculates the local timeout interval based on how many rounds have failed and keeps track
	// of validator data related to the active round (votes received if next leader or votes sent if follower).
	RoundState struct {
		// Last commit.
		highCommittedRound uint64
		// Current round is max(highest QC, highest TC) + 1.
		currentRound uint64
		// The deadline for the next local timeout event. It is reset every time a new round start, or
		// a previous deadline expires.
		roundTimeout time.Time
		// timeout calculator
		timeoutCalculator TimeoutCalculator
		// Collection of votes (when node is the next leader)
		pendingVotes *VoteRegister
		// Last round timeout certificate
		lastRoundTC *atomic_broadcast.TimeoutCert
		// Vote sent locally for the current round.
		voteSent *atomic_broadcast.VoteMsg
	}
)

func (x *RoundState) LastRoundTC() *atomic_broadcast.TimeoutCert {
	return x.lastRoundTC
}

func min(x, y uint64) uint64 {
	if x < y {
		return x
	}
	return y
}

func (x ExponentialTimeInterval) GetNextTimeout(roundsAfterLastCommit uint64) time.Duration {
	exp := min(uint64(x.maxExponent), roundsAfterLastCommit)
	mul := math.Pow(x.exponentBase, float64(exp))
	return time.Duration(float64(x.baseMs) * mul)
}

// NewRoundState Needs to be constructed from last QC!
func NewRoundState(lastRound uint64, localTimeout time.Duration) *RoundState {
	return &RoundState{
		highCommittedRound: lastRound,
		currentRound:       lastRound + 1,
		roundTimeout:       time.Now().Add(localTimeout),
		timeoutCalculator:  ExponentialTimeInterval{baseMs: localTimeout, exponentBase: 1.2, maxExponent: 0},
		pendingVotes:       NewVoteRegister(),
		lastRoundTC:        nil,
		voteSent:           nil,
	}
}

func (x *RoundState) GetCurrentRound() uint64 {
	return x.currentRound
}

func (x *RoundState) SetVoted(vote *atomic_broadcast.VoteMsg) {
	if vote.VoteInfo.RootRound == x.currentRound {
		x.voteSent = vote
	}
}
func (x *RoundState) GetVoted() *atomic_broadcast.VoteMsg {
	return x.voteSent
}

func (x *RoundState) GetRoundTimeout() time.Duration {
	return x.roundTimeout.Sub(time.Now())
}

func (x *RoundState) RegisterVote(vote *atomic_broadcast.VoteMsg, quorum QuorumInfo) (*atomic_broadcast.QuorumCert, *atomic_broadcast.TimeoutCert) {
	// If the vote is not about the current round then ignore
	if vote.VoteInfo.RootRound != x.currentRound {
		logger.Warning("Round %v received vote for unexpected round %v: vote ignored",
			x.currentRound, vote.VoteInfo.RootRound)
		return nil, nil
	}
	qc, tc, err := x.pendingVotes.InsertVote(vote, quorum)
	if err != nil {
		logger.Warning("Round %v vote message from %v error:", x.currentRound, vote.Author, err)
		return nil, nil
	}
	return qc, tc
}

func (x *RoundState) AdvanceRoundQC(qc *atomic_broadcast.QuorumCert) bool {
	if qc == nil {
		return false
	}
	if qc.VoteInfo.RootRound < x.currentRound {
		return false
	}
	// last vote is now obsolete
	x.voteSent = nil
	x.lastRoundTC = nil
	x.highCommittedRound = qc.VoteInfo.RootRound
	x.startNewRound(qc.VoteInfo.RootRound + 1)
	return true
}

func (x *RoundState) AdvanceRoundTC(tc *atomic_broadcast.TimeoutCert) {
	// no timeout cert or is from old view/round - ignore
	if tc == nil || tc.Timeout.Round < x.currentRound {
		return
	}
	// last vote is now obsolete
	x.voteSent = nil
	x.lastRoundTC = tc
	x.startNewRound(tc.Timeout.Round + 1)
}

func (x *RoundState) startNewRound(round uint64) {
	x.currentRound = round
	x.roundTimeout = time.Now().Add(x.timeoutCalculator.GetNextTimeout(x.currentRound - x.highCommittedRound - 1))
}
