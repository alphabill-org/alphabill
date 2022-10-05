package rootvalidator

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

type SafetyModule struct {
	highestVotedRound uint64
	highestQcRound    uint64
	privateKey        crypto.Signer
}

func max(a, b uint64) uint64 {
	if a < b {
		return b
	}
	return a
}

func IsConsecutive(blockRound, round uint64) bool {
	return round+1 == blockRound
}

func isSaveToExtend(blockRound, qcRound uint64, tc *TimeoutCertificate) bool {
	return IsConsecutive(blockRound, tc.GetRound()) && qcRound >= tc.timeout.GetQcRound()
}

func NewSafetyModule(signer crypto.Signer) *SafetyModule {
	return &SafetyModule{highestVotedRound: 0, highestQcRound: 0, privateKey: signer}
}

func (s *SafetyModule) IsSafeToVote(blockRound, qcRound uint64, tc *TimeoutCertificate) bool {
	if blockRound <= max(s.highestVotedRound, qcRound) {
		// 1. must vote in monotonically increasing rounds
		// 2. must extend a smaller round
		return false
	}
	// Extending qc from previous round or safe to extend due to tc
	return IsConsecutive(blockRound, qcRound) || isSaveToExtend(blockRound, qcRound, tc)
}

func (s SafetyModule) SignVote() error {
	//TODO implement me
	panic("implement me")
}

func (s SafetyModule) SignTimeout(timeout *atomic_broadcast.Timeout, certificate *TimeoutCertificate) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (s SafetyModule) SignProposal() error {
	//TODO implement me
	panic("implement me")
}

func (s *SafetyModule) increaseHigestVoteRound(round uint64) {
	s.highestVotedRound = max(s.highestVotedRound, round)
}

func (s *SafetyModule) updateHighestQcRound(qcRound uint64) {
	s.highestQcRound = max(s.highestQcRound, qcRound)
}

func (s *SafetyModule) isSafeToTimeout(round, qcRound uint64, tc *TimeoutCertificate) bool {
	if qcRound < s.highestQcRound || round <= max(s.highestVotedRound-1, qcRound) {
		// respect highest qc round and don’t timeout in a past round
		return false
	}
	// qc or tc must allow entering the round to timeout
	return IsConsecutive(round, qcRound) || IsConsecutive(round, tc.GetRound())
}
