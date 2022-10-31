package rootvalidator

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
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

func isSaveToExtend(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	return IsConsecutive(blockRound, tc.Timeout.Round) && qcRound >= tc.Timeout.Hqc.VoteInfo.Round //  GetQcRound()
}

func NewSafetyModule(signer crypto.Signer) *SafetyModule {
	return &SafetyModule{highestVotedRound: 0, highestQcRound: 0, privateKey: signer}
}

func (s *SafetyModule) IsSafeToVote(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if blockRound <= max(s.highestVotedRound, qcRound) {
		// 1. must vote in monotonically increasing rounds
		// 2. must extend a smaller round
		return false
	}
	// Extending qc from previous round or safe to extend due to tc
	if tc != nil {
		return IsConsecutive(blockRound, qcRound) || isSaveToExtend(blockRound, qcRound, tc)
	}
	return IsConsecutive(blockRound, qcRound)
}

func (s SafetyModule) SignVote(msg *atomic_broadcast.VoteMsg, lastRoundTC *atomic_broadcast.TimeoutCert) error {
	qcRound := msg.VoteInfo.ParentRound
	votingRound := msg.VoteInfo.Round
	if s.IsSafeToVote(votingRound, qcRound, lastRoundTC) == false {
		return errors.New("Not safe to vote")
	}
	s.updateHighestQcRound(qcRound)
	s.increaseHigestVoteRound(votingRound)
	// Attach commit info and vote
	msg.CommitInfo = atomic_broadcast.NewCommitInfo(s.isCommitCandidate(msg.VoteInfo), msg.VoteInfo, gocrypto.SHA256)
	// signs commit info hash
	if err := msg.AddSignature(s.privateKey); err != nil {
		return err
	}
	return nil
}

func (s SafetyModule) SignTimeout(timeout *atomic_broadcast.Timeout, lastRoundTC *atomic_broadcast.TimeoutCert) ([]byte, error) {
	qcRound := timeout.Hqc.VoteInfo.Round
	round := timeout.Round
	if !s.isSafeToTimeout(round, qcRound, lastRoundTC) {
		return nil, errors.New("not safe to timeout")
	}
	// stop voting for this round, all other request to sign a vote for this round will be rejected
	s.increaseHigestVoteRound(round)
	// Sign timeout
	signTimeout := atomic_broadcast.NewTimeoutSign(round, timeout.GetEpoch(), qcRound)
	signature, err := s.privateKey.SignHash(signTimeout.Hash(gocrypto.SHA256))
	if err != nil {
		return nil, err
	}
	return signature, nil
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

func (s *SafetyModule) isSafeToTimeout(round, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if qcRound < s.highestQcRound || round <= max(s.highestVotedRound-1, qcRound) {
		// respect highest qc round and don’t timeout in a past round
		return false
	}
	// qc or tc must allow entering the round to timeout
	return IsConsecutive(round, qcRound) || IsConsecutive(round, tc.Timeout.Round)
}

func (s *SafetyModule) isCommitCandidate(info *atomic_broadcast.VoteInfo) []byte {
	if info == nil {
		return nil
	}
	if IsConsecutive(info.Round, info.ParentRound) {
		return info.ExecStateId
	}
	return nil
}
