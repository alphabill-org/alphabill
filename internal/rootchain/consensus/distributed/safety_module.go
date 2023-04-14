package distributed

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
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

func (s *SafetyModule) isSafeToVote(block *ab_consensus.BlockData, lastRoundTC *ab_consensus.TimeoutCert) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}
	blockRound := block.Round
	// never vote for the same round twice
	if blockRound <= s.GetHighestVotedRound() {
		return fmt.Errorf("already voted for round %d, last voted round %d",
			blockRound, s.GetHighestVotedRound())
	}
	qcRound := block.Qc.GetRound()
	// normal case, block is extended from last QC
	if lastRoundTC == nil {
		if !isConsecutive(blockRound, qcRound) {
			return fmt.Errorf("block round %d does not extend from block qc round %d", blockRound, qcRound)
		}
		// all is fine
		return nil
	}
	// previous round was timeout, block is extended from TC
	tcRound := lastRoundTC.GetRound()
	tcHqcRound := lastRoundTC.GetHqcRound()
	if !isConsecutive(blockRound, tcRound) {
		return fmt.Errorf("block round %d does not extend timeout certificate round %d",
			blockRound, tcRound)
	}
	if qcRound < tcHqcRound {
		return fmt.Errorf("block qc round %d is smaller than timeout certificate highest qc round %d",
			qcRound, tcHqcRound)
	}
	return nil
}

func (s *SafetyModule) constructCommitInfo(block *ab_consensus.BlockData, voteInfoHash []byte) *certificates.CommitInfo {
	commitHash := s.isCommitCandidate(block)
	return &certificates.CommitInfo{RootRoundInfoHash: voteInfoHash, RootHash: commitHash}
}

func (s *SafetyModule) MakeVote(block *ab_consensus.BlockData, execStateID []byte, highQC *ab_consensus.QuorumCert, lastRoundTC *ab_consensus.TimeoutCert) (*ab_consensus.VoteMsg, error) {
	// The overall validity of the block must be checked prior to calling this method
	// However since we are de-referencing QC make sure it is not nil
	if block.Qc == nil {
		return nil, fmt.Errorf("make vote error, block is missing quorum certificate")
	}
	qcRound := block.Qc.VoteInfo.RoundNumber
	votingRound := block.Round
	if err := s.isSafeToVote(block, lastRoundTC); err != nil {
		return nil, fmt.Errorf("not safe to vote, %w", err)
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
	voteMsg := &ab_consensus.VoteMsg{
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

func (s *SafetyModule) SignTimeout(tmoVote *ab_consensus.TimeoutMsg, lastRoundTC *ab_consensus.TimeoutCert) error {
	if err := tmoVote.IsValid(); err != nil {
		return fmt.Errorf("timeout message not valid, %w", err)
	}
	qcRound := tmoVote.Timeout.HighQc.GetRound()
	round := tmoVote.GetRound()
	if err := s.isSafeToTimeout(round, qcRound, lastRoundTC); err != nil {
		return fmt.Errorf("not safe to time-out, %w", err)
	}
	// stop voting for this round, all other request to sign a normal vote for this round will be rejected
	s.increaseHigestVoteRound(round)
	// Sign timeout
	return tmoVote.Sign(s.signer)
}

func (s *SafetyModule) SignProposal(proposalMsg *ab_consensus.ProposalMsg) error {
	return proposalMsg.Sign(s.signer)
}

func (s *SafetyModule) increaseHigestVoteRound(round uint64) {
	s.SetHighestVotedRound(util.Max(s.GetHighestVotedRound(), round))
}

func (s *SafetyModule) updateHighestQcRound(qcRound uint64) {
	s.SetHighestQcRound(util.Max(s.GetHighestQcRound(), qcRound))
}

func (s *SafetyModule) isSafeToTimeout(round, qcRound uint64, lastRoundTC *ab_consensus.TimeoutCert) error {
	if qcRound < s.GetHighestQcRound() {
		// respect highest qc round
		return fmt.Errorf("qc round %v is smaller than highest qc round %v seen", qcRound, s.GetHighestQcRound())
	}
	highestVotedRound := s.GetHighestVotedRound() - 1
	if round <= util.Max(highestVotedRound, qcRound) {
		// don’t time out in a past round
		return fmt.Errorf("timeout round %v is in the past, highest voted round %v, hqc round %v",
			round, highestVotedRound, qcRound)
	}
	var tcRound uint64 = 0
	if lastRoundTC != nil {
		tcRound = lastRoundTC.GetRound()
	}
	// timeout round must follow either last qc or tc
	if !isConsecutive(round, qcRound) && !isConsecutive(round, tcRound) {
		return fmt.Errorf("round %v does not follow last qc round %v or tc round %v",
			round, qcRound, tcRound)
	}
	return nil
}

func (s *SafetyModule) isCommitCandidate(block *ab_consensus.BlockData) []byte {
	if block.Qc == nil {
		return nil
	}
	// next round after genesis block is not a commit round
	if isConsecutive(block.Round, block.Qc.VoteInfo.RoundNumber) {
		return block.Qc.VoteInfo.CurrentRootHash
	}
	return nil
}
