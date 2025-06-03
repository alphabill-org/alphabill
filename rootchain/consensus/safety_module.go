package consensus

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	SafetyModule struct {
		network  types.NetworkID
		peerID   string
		signer   crypto.Signer
		verifier crypto.Verifier
		storage  SafetyStorage
	}

	Signable interface {
		Sign(s crypto.Signer) error
	}

	// Persistent storage for SafetyModule state
	SafetyStorage = interface {
		GetHighestVotedRound() uint64
		SetHighestVotedRound(uint64) error
		GetHighestQcRound() uint64
		SetHighestQcRound(qcRound, votedRound uint64) error
	}
)

func isConsecutive(blockRound, round uint64) bool {
	return round+1 == blockRound
}

func NewSafetyModule(network types.NetworkID, id string, signer crypto.Signer, db SafetyStorage) (*SafetyModule, error) {
	ver, err := signer.Verifier()
	if err != nil {
		return nil, fmt.Errorf("invalid root validator signing key: %w", err)
	}

	return &SafetyModule{network: network, peerID: id, signer: signer, verifier: ver, storage: db}, nil
}

func (s *SafetyModule) isSafeToVote(block *drctypes.BlockData, lastRoundTC *drctypes.TimeoutCert) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}
	blockRound := block.Round
	// never vote for the same round twice
	if hvr := s.storage.GetHighestVotedRound(); blockRound <= hvr {
		return fmt.Errorf("already voted for round %d, last voted round %d", blockRound, hvr)
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

func (s *SafetyModule) constructCommitInfo(block *drctypes.BlockData, voteInfoHash []byte) *types.UnicitySeal {
	committedRound := s.isCommitCandidate(block)
	if committedRound == nil {
		return &types.UnicitySeal{Version: 1, PreviousHash: voteInfoHash}
	}
	return &types.UnicitySeal{
		Version:              1,
		NetworkID:            s.network,
		PreviousHash:         voteInfoHash,
		RootChainRoundNumber: committedRound.RoundNumber,
		Epoch:                committedRound.Epoch,
		Timestamp:            committedRound.Timestamp,
		Hash:                 committedRound.CurrentRootHash,
	}
}

func (s *SafetyModule) MakeVote(block *drctypes.BlockData, execStateID []byte, highQC *drctypes.QuorumCert, lastRoundTC *drctypes.TimeoutCert) (*abdrc.VoteMsg, error) {
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
	if err := s.storage.SetHighestQcRound(qcRound, votingRound); err != nil {
		return nil, fmt.Errorf("persisting voting rounds: %w", err)
	}

	// create vote info
	voteInfo := &drctypes.RoundInfo{
		RoundNumber:       block.Round,
		Epoch:             block.Epoch,
		Timestamp:         block.Timestamp,
		ParentRoundNumber: block.Qc.VoteInfo.RoundNumber,
		CurrentRootHash:   execStateID,
	}
	h, err := voteInfo.Hash(gocrypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to hash vote info: %w", err)
	}
	// Create ledger commit info, the signed part of vote
	ledgerCommitInfo := s.constructCommitInfo(block, h)
	voteMsg := &abdrc.VoteMsg{
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

func (s *SafetyModule) SignTimeout(tmoVote *abdrc.TimeoutMsg, lastRoundTC *drctypes.TimeoutCert) error {
	if err := tmoVote.IsValid(); err != nil {
		return fmt.Errorf("timeout message not valid, %w", err)
	}
	qcRound := tmoVote.Timeout.HighQc.GetRound()
	round := tmoVote.GetRound()
	if err := s.isSafeToTimeout(round, qcRound, lastRoundTC); err != nil {
		return fmt.Errorf("not safe to time-out, %w", err)
	}
	// stop voting for this round, all other request to sign a normal vote for this round will be rejected
	if err := s.storage.SetHighestVotedRound(round); err != nil {
		return fmt.Errorf("storing voted round: %w", err)
	}
	// Sign timeout
	return tmoVote.Sign(s.signer)
}

func (s *SafetyModule) Sign(msg Signable) error {
	return msg.Sign(s.signer)
}

func (s *SafetyModule) isSafeToTimeout(round, tmoHighQCRound uint64, lastRoundTC *drctypes.TimeoutCert) error {
	if hqc := s.storage.GetHighestQcRound(); tmoHighQCRound < hqc {
		// respect highest qc round
		return fmt.Errorf("timeout high qc round %d is smaller than highest qc round %d seen", tmoHighQCRound, hqc)
	}
	if round <= tmoHighQCRound {
		return fmt.Errorf("timeout round %v is in the past, timeout msg high qc is for round %v",
			round, tmoHighQCRound)
	}
	if hvr := s.storage.GetHighestVotedRound(); round < hvr {
		// don’t time out in a past round
		return fmt.Errorf("timeout round %d is in the past, already signed vote for round %d", round, hvr)
	}
	var tcRound uint64 = 0
	if lastRoundTC != nil {
		tcRound = lastRoundTC.GetRound()
	}
	// timeout round must follow either last qc or tc
	if !isConsecutive(round, tmoHighQCRound) && !isConsecutive(round, tcRound) {
		return fmt.Errorf("round %v does not follow last qc round %v or tc round %v",
			round, tmoHighQCRound, tcRound)
	}
	return nil
}

// isCommitCandidate - returns committed round info if commit criteria is valid
func (s *SafetyModule) isCommitCandidate(block *drctypes.BlockData) *drctypes.RoundInfo {
	if block.Qc == nil {
		return nil
	}
	// consecutive successful round commits previous round
	if isConsecutive(block.Round, block.Qc.VoteInfo.RoundNumber) {
		return block.Qc.VoteInfo
	}
	return nil
}
