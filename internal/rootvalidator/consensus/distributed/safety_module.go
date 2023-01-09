package distributed

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

type SafetyModule struct {
	highestVotedRound uint64
	highestQcRound    uint64
	signer            crypto.Signer
	verifier          crypto.Verifier
}

func max(a, b uint64) uint64 {
	if a < b {
		return b
	}
	return a
}

func isConsecutive(blockRound, round uint64) bool {
	return round+1 == blockRound
}

func isSafeToExtend(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if !isConsecutive(blockRound, tc.Timeout.Round) {
		return false
	}
	if qcRound < tc.Timeout.HighQc.VoteInfo.RoundNumber {
		return false
	}
	//return isConsecutive(blockRound, tc.Timeout.Round) && qcRound >= tc.Timeout.Hqc.VoteInfo.RootRound
	return true
}

func NewSafetyModule(signer crypto.Signer) (*SafetyModule, error) {
	ver, err := signer.Verifier()
	if err != nil {
		return nil, fmt.Errorf("invalid root validator sign key: %w", err)
	}

	return &SafetyModule{highestVotedRound: 0, highestQcRound: 0, signer: signer, verifier: ver}, nil
}

func (s *SafetyModule) isSafeToVote(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if blockRound <= max(s.highestVotedRound, qcRound) {
		// 1. must vote in monotonically increasing rounds
		// 2. must extend a smaller round
		return false
	}
	// Either extending from previous round QC or safe to extend due to last round TC
	if tc != nil {
		return isConsecutive(blockRound, qcRound) || isSafeToExtend(blockRound, qcRound, tc)
	}
	return isConsecutive(blockRound, qcRound)
}

func (s *SafetyModule) constructCommitInfo(block *atomic_broadcast.BlockData, voteInfoHash []byte) *certificates.CommitInfo {
	commitHash := s.isCommitCandidate(block)
	return &certificates.CommitInfo{RootRoundInfoHash: voteInfoHash, RootHash: commitHash}
}

func (s *SafetyModule) MakeVote(block *atomic_broadcast.BlockData, execStateId []byte, author string, lastRoundTC *atomic_broadcast.TimeoutCert) (*atomic_broadcast.VoteMsg, error) {
	// The overall validity of the block must be checked prior to calling this method
	// However since we are de-referencing QC make sure it is not nil
	if block.Qc == nil {
		return nil, atomic_broadcast.ErrMissingQuorumCertificate
	}
	qcRound := block.Qc.VoteInfo.RoundNumber
	votingRound := block.Round
	if s.isSafeToVote(votingRound, qcRound, lastRoundTC) == false {
		return nil, errors.New("Not safe to vote")
	}
	s.updateHighestQcRound(qcRound)
	s.increaseHigestVoteRound(votingRound)
	// create vote info
	voteInfo := &certificates.RootRoundInfo{
		RoundNumber:       block.Round,
		Epoch:             block.Epoch,
		Timestamp:         block.Timestamp,
		ParentRoundNumber: block.Qc.VoteInfo.RoundNumber,
		CurrentRootHash:   execStateId,
	}
	// Create ledger commit info, the signed part of vote
	ledgerCommitInfo := s.constructCommitInfo(block, voteInfo.Hash(gocrypto.SHA256))
	voteMsg := &atomic_broadcast.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: ledgerCommitInfo,
		HighQc:           block.Qc,
		Author:           author,
	}
	// signs commit info hash
	if err := voteMsg.Sign(s.signer); err != nil {
		return nil, err
	}
	return voteMsg, nil
}

func (s SafetyModule) SignTimeout(vote *atomic_broadcast.TimeoutMsg, lastRoundTC *atomic_broadcast.TimeoutCert) error {
	qcRound := vote.Timeout.HighQc.VoteInfo.RoundNumber
	round := vote.Timeout.Round
	if !s.isSafeToTimeout(round, qcRound, lastRoundTC) {
		return errors.New("not safe to timeout")
	}
	// stop voting for this round, all other request to sign a vote for this round will be rejected
	s.increaseHigestVoteRound(round)
	// Sign timeout
	return vote.Sign(s.signer)
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
	var tcRound uint64 = 0
	if tc != nil {
		tcRound = tc.Timeout.Round
	}
	if qcRound < s.highestQcRound || round <= max(s.highestVotedRound-1, qcRound) {
		// respect highest qc round and don’t timeout in a past round
		return false
	}
	// qc or tc must allow entering the round to timeout
	return isConsecutive(round, qcRound) || isConsecutive(round, tcRound)
}

func (s *SafetyModule) isCommitCandidate(block *atomic_broadcast.BlockData) []byte {
	if block.Qc == nil {
		return nil
	}
	if isConsecutive(block.Round, block.Qc.VoteInfo.RoundNumber) {
		return block.Qc.VoteInfo.CurrentRootHash
	}
	return nil
}

func (s *SafetyModule) GetVerifier() crypto.Verifier {
	return s.verifier
}
