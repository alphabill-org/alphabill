package distributed

import (
	"math"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
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
		lastRoundTC *ab_consensus.TimeoutCert
		// Store for votes sent in the ongoing round.
		voteSent    *ab_consensus.VoteMsg
		timeoutVote *ab_consensus.TimeoutMsg
	}
)

func (x *Pacemaker) LastRoundTC() *ab_consensus.TimeoutCert {
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

// SetVoted - remember vote sent in this view
func (x *Pacemaker) SetVoted(vote *ab_consensus.VoteMsg) {
	if vote.VoteInfo.RoundNumber == x.currentRound {
		x.voteSent = vote
	}
}

// GetVoted - has the node voted in this round, returns either vote or nil
func (x *Pacemaker) GetVoted() *ab_consensus.VoteMsg {
	return x.voteSent
}

// SetTimeoutVote - remember timeout vote sent in this view
func (x *Pacemaker) SetTimeoutVote(vote *ab_consensus.TimeoutMsg) {
	if vote.Timeout.Round == x.currentRound {
		x.timeoutVote = vote
	}
}

// GetTimeoutVote - has the node voted for timeout in this round, returns either vote or nil
func (x *Pacemaker) GetTimeoutVote() *ab_consensus.TimeoutMsg {
	return x.timeoutVote
}

// GetRoundTimeout - get round local timeout
func (x *Pacemaker) GetRoundTimeout() time.Duration {
	return x.roundTimeout.Sub(time.Now())
}

// CalcTimeTilNextProposal calculates delay for next round symmetrically, no round
// should take less than half of block rate - two consecutive rounds not less than block rate
func (x *Pacemaker) CalcTimeTilNextProposal() time.Duration {
	//symmetric delay
	now := time.Now()
	if now.Sub(x.lastViewChange) >= x.blockRate/2 {
		return 0
	}
	return x.lastViewChange.Add(x.blockRate / 2).Sub(now)
}

/*
// CalcAsyncTimeTilNextProposal - delays every second round, 2 consecutive rounds
// must never be faster that block rate.
func (x *Pacemaker) CalcAsyncTimeTilNextProposal(round uint64) time.Duration {
	now := time.Now()
	// according to spec. in case of 2-chain-rule finality the wait is every 2nd block
	if now.Sub(x.lastViewChange) >= time.Duration(round%2)*x.blockRate {
		return 0
	}
	return x.lastViewChange.Add(x.blockRate).Sub(now)
}
*/

// RegisterVote - register vote from another root node, this node is the leader and assembles votes for quorum certificate
func (x *Pacemaker) RegisterVote(vote *ab_consensus.VoteMsg, quorum QuorumInfo) *ab_consensus.QuorumCert {
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

// RegisterTimeoutVote - register time-out vote from another root node, this node is the leader and tries to assemble
// a timeout quorum certificate for this round
func (x *Pacemaker) RegisterTimeoutVote(vote *ab_consensus.TimeoutMsg, quorum QuorumInfo) *ab_consensus.TimeoutCert {
	tc, err := x.pendingVotes.InsertTimeoutVote(vote, quorum)
	if err != nil {
		logger.Warning("Round %v vote message from %v error:", x.currentRound, vote.Author, err)
		return nil
	}
	return tc
}

// AdvanceRoundQC - trigger next round/view on quorum certificate
func (x *Pacemaker) AdvanceRoundQC(qc *ab_consensus.QuorumCert) bool {
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

// AdvanceRoundTC - trigger next round/view on timeout certificate
func (x *Pacemaker) AdvanceRoundTC(tc *ab_consensus.TimeoutCert) {
	// no timeout cert or is from old view/round - ignore
	if tc == nil || tc.Timeout.Round < x.currentRound {
		return
	}
	x.lastRoundTC = tc
	x.startNewRound(tc.Timeout.Round + 1)
}

// startNewRound - starts new round, sets new round number, resets all stores and
// calculates the new round timeout
func (x *Pacemaker) startNewRound(round uint64) {
	x.clear()
	x.lastViewChange = time.Now()
	x.currentRound = round
	x.pendingVotes.Reset()
	x.roundTimeout = time.Now().Add(x.timeoutCalculator.GetNextTimeout(x.currentRound - x.lastQcToCommitRound - 1))
}
