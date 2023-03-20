package distributed

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	// genesis state is certified with rounds 1
	defaultHighestVotedRound = 1
	defaultHighestQcRound    = 1
	highestVotedKey          = "votedRound"
	highestQcKey             = "qcRound"
)

type (
	SafetyModule struct {
		peerID   string
		signer   crypto.Signer
		verifier crypto.Verifier
		storage  keyvaluedb.KeyValueDB
	}
)

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
	return true
}

func NewSafetyModule(id string, signer crypto.Signer, s keyvaluedb.KeyValueDB) (*SafetyModule, error) {
	ver, err := signer.Verifier()
	if err != nil {
		return nil, fmt.Errorf("invalid root validator sign key: %w", err)
	}

	return &SafetyModule{peerID: id, signer: signer, verifier: ver, storage: s}, nil
}

func (s *SafetyModule) GetHighestVotedRound() uint64 {
	var hVoteRound uint64 = defaultHighestVotedRound
	found, err := s.storage.Read([]byte(highestVotedKey), &hVoteRound)
	if !found || err != nil {
		return defaultHighestVotedRound
	}
	return hVoteRound
}

func (s *SafetyModule) SetHighestVotedRound(highestVotedRound uint64) {
	_ = s.storage.Write([]byte(highestVotedKey), &highestVotedRound)
}

func (s *SafetyModule) GetHighestQcRound() uint64 {
	var qcRound uint64 = defaultHighestQcRound
	found, err := s.storage.Read([]byte(highestQcKey), &qcRound)
	if !found || err != nil {
		return defaultHighestQcRound
	}
	return qcRound
}

func (s *SafetyModule) SetHighestQcRound(highestQcRound uint64) {
	_ = s.storage.Write([]byte(highestQcKey), &highestQcRound)
}

func (s *SafetyModule) isSafeToVote(blockRound, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	if blockRound <= util.Max(s.GetHighestVotedRound(), qcRound) {
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

func (s *SafetyModule) MakeVote(block *atomic_broadcast.BlockData, execStateID []byte, highQC *atomic_broadcast.QuorumCert, lastRoundTC *atomic_broadcast.TimeoutCert) (*atomic_broadcast.VoteMsg, error) {
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
		CurrentRootHash:   execStateID,
	}
	// Create ledger commit info, the signed part of vote
	ledgerCommitInfo := s.constructCommitInfo(block, voteInfo.Hash(gocrypto.SHA256))
	voteMsg := &atomic_broadcast.VoteMsg{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: ledgerCommitInfo,
		HighQc:           highQC,
		Author:           s.peerID,
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
	s.SetHighestVotedRound(util.Max(s.GetHighestVotedRound(), round))
}

func (s *SafetyModule) updateHighestQcRound(qcRound uint64) {
	s.SetHighestQcRound(util.Max(s.GetHighestQcRound(), qcRound))
}

func (s *SafetyModule) isSafeToTimeout(round, qcRound uint64, tc *atomic_broadcast.TimeoutCert) bool {
	var tcRound uint64 = 0
	var highestVotedRound uint64 = 0
	if tc != nil {
		tcRound = tc.Timeout.Round
	}
	if s.GetHighestVotedRound() > 0 {
		highestVotedRound = s.GetHighestVotedRound() - 1
	}
	if qcRound < s.GetHighestQcRound() || round <= util.Max(highestVotedRound, qcRound) {
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
	// next round after genesis block is not a commit round
	if isConsecutive(block.Round, block.Qc.VoteInfo.RoundNumber) {
		return block.Qc.VoteInfo.CurrentRootHash
	}
	return nil
}
