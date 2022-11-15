package distributed

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

type SafetyModule struct {
	highestVotedRound uint64
	highestQcRound    uint64
	signer            crypto.Signer
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
	if !IsConsecutive(blockRound, tc.Timeout.Round) {
		return false
	}
	if qcRound < tc.Timeout.Hqc.VoteInfo.RootRound {
		return false
	}
	//return IsConsecutive(blockRound, tc.Timeout.Round) && qcRound >= tc.Timeout.Hqc.VoteInfo.RootRound
	return true
}

func NewSafetyModule(signer crypto.Signer) *SafetyModule {
	return &SafetyModule{highestVotedRound: 0, highestQcRound: 0, signer: signer}
}

func (s *SafetyModule) IsSafeToVote(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if blockRound <= max(s.highestVotedRound, qcRound) {
		// 1. must vote in monotonically increasing rounds
		// 2. must extend a smaller round
		return false
	}
	// Either extending from previous round QC or safe to extend due to last round TC
	if tc != nil {
		return IsConsecutive(blockRound, qcRound) || isSaveToExtend(blockRound, qcRound, tc)
	}
	return IsConsecutive(blockRound, qcRound)
}

func (s *SafetyModule) constructLedgerCommitInfo(block *atomic_broadcast.BlockData, voteInfoHash []byte) *atomic_broadcast.LedgerCommitInfo {
	commitCandidateId := s.isCommitCandidate(block)
	return &atomic_broadcast.LedgerCommitInfo{CommitStateId: commitCandidateId, VoteInfoHash: voteInfoHash}
}

func (s *SafetyModule) MakeVote(block *atomic_broadcast.BlockData, execStateId []byte, author string, lastRoundTC *atomic_broadcast.TimeoutCert) (*atomic_broadcast.VoteMsg, error) {
	qcRound := block.Qc.VoteInfo.RootRound
	votingRound := block.Round
	if s.IsSafeToVote(votingRound, qcRound, lastRoundTC) == false {
		return nil, errors.New("Not safe to vote")
	}
	s.updateHighestQcRound(qcRound)
	s.increaseHigestVoteRound(votingRound)
	// create vote info
	voteInfo := &atomic_broadcast.VoteInfo{
		BlockId:       block.Id,
		RootRound:     block.Round,
		Epoch:         block.Epoch,
		Timestamp:     block.Timestamp,
		ParentBlockId: block.Qc.VoteInfo.BlockId,
		ParentRound:   block.Qc.VoteInfo.RootRound,
		ExecStateId:   execStateId,
	}
	// Create ledger commit info, the signed part of vote
	ledgerCommitInfo := s.constructLedgerCommitInfo(block, voteInfo.Hash(gocrypto.SHA256))
	voteMsg := &atomic_broadcast.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: ledgerCommitInfo,
		HighCommitQc:     nil,
		Author:           author,
		TimeoutSignature: nil,
	}
	// signs commit info hash
	if err := voteMsg.AddSignature(s.signer); err != nil {
		return nil, err
	}
	return voteMsg, nil
}

func (s SafetyModule) SignTimeout(timeout *atomic_broadcast.Timeout, lastRoundTC *atomic_broadcast.TimeoutCert) ([]byte, error) {
	qcRound := timeout.Hqc.VoteInfo.RootRound
	round := timeout.Round
	if !s.isSafeToTimeout(round, qcRound, lastRoundTC) {
		return nil, errors.New("not safe to timeout")
	}
	// stop voting for this round, all other request to sign a vote for this round will be rejected
	s.increaseHigestVoteRound(round)
	// Sign timeout
	signTimeout := atomic_broadcast.NewTimeoutSign(round, timeout.GetEpoch(), qcRound)
	signature, err := s.signer.SignHash(signTimeout.Hash(gocrypto.SHA256))
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (s SafetyModule) SignProposal(proposalMsg *atomic_broadcast.ProposalMsg) error {
	return proposalMsg.Sign(s.signer)
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

func (s *SafetyModule) isCommitCandidate(block *atomic_broadcast.BlockData) []byte {
	if block.Qc == nil {
		return nil
	}
	if IsConsecutive(block.Round, block.Qc.VoteInfo.RootRound) {
		return block.Qc.VoteInfo.ExecStateId
	}
	return nil
}
