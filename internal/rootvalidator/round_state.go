package rootvalidator

import (
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

type (
	TimeoutCalculator interface {
		GetNextTimeout(roundIndexAfterCommit uint64) time.Duration
	}

	ExponentialTimeInterval struct {
		baseMs   time.Duration
		exponent float64
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

func (x ExponentialTimeInterval) GetNextTimeout(roundIndexAfterCommit uint64) time.Duration {
	// Not the correct equation yet
	return x.baseMs + (400 * time.Duration(roundIndexAfterCommit) * time.Millisecond)
}

// NewRoundState Needs to be constructed from last QC!
func NewRoundState(localTimeout time.Duration) *RoundState {
	return &RoundState{
		highCommittedRound: 0,
		currentRound:       0,
		roundTimeout:       time.Now(),
		timeoutCalculator:  ExponentialTimeInterval{baseMs: localTimeout, exponent: 0},
		pendingVotes:       NewVoteRegister(),
		lastRoundTC:        nil,
		voteSent:           nil,
	}
}

func (x *RoundState) GetCurrentRound() uint64 {
	return x.currentRound
}

func (x *RoundState) SetVoted(vote *atomic_broadcast.VoteMsg) {
	if vote.VoteInfo.Proposed.Round == x.currentRound {
		x.voteSent = vote
	}
}
func (x *RoundState) GetVoted() *atomic_broadcast.VoteMsg {
	return x.voteSent
}

func (x *RoundState) GetRoundTimeout() time.Duration {
	newTimeout := x.timeoutCalculator.GetNextTimeout(x.highCommittedRound)
	x.roundTimeout = time.Now().Add(newTimeout)
	return newTimeout
}

func (x *RoundState) RegisterVote(vote *atomic_broadcast.VoteMsg, verifier *RootNodeVerifier) (*atomic_broadcast.QuorumCert, *atomic_broadcast.TimeoutCert) {
	// If the vote is not about the current round then ignore
	if vote.VoteInfo.Proposed.Round != x.currentRound {
		logger.Warning("Round %v received vote for unexpected round %v: vote ignored",
			x.currentRound, vote.VoteInfo.Proposed.Round)
		return nil, nil
	}
	return x.pendingVotes.InsertVote(vote, verifier)
}

func (x *RoundState) AdvanceRoundQC(qc *atomic_broadcast.QuorumCert) bool {
	if qc.VoteInfo.Proposed.Round < x.currentRound {
		return false
	}
	// last vote is now obsolete
	x.voteSent = nil
	x.lastRoundTC = nil
	x.currentRound = qc.VoteInfo.Proposed.Round + 1
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
	x.currentRound = tc.Timeout.Round + 1
}
