package consensus

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	QuorumInfo interface {
		GetQuorumThreshold() uint64
		GetMaxFaultyNodes() uint64
	}

	ConsensusWithSignatures struct {
		voteInfo   *drctypes.RoundInfo
		commitInfo *types.UnicitySeal
		signatures map[string]hex.Bytes
	}

	VoteRegister struct {
		// Hash of ConsensusInfo to signatures/votes for it
		hashToSignatures map[voteID]*ConsensusWithSignatures
		// Tracks all timeout votes for this round
		// if 2f+1 or threshold votes, then TC is formed
		timeoutCert *drctypes.TimeoutCert
		// Helper, to avoid duplicate votes
		authorToVote map[string]voteID
	}

	// sha256 hash is used to create ID for a vote
	voteID = [sha256.Size]byte
)

var ErrVoteIsNil = errors.New("vote is nil")

func NewVoteRegister() *VoteRegister {
	return &VoteRegister{
		hashToSignatures: make(map[voteID]*ConsensusWithSignatures),
		timeoutCert:      nil,
		authorToVote:     make(map[string]voteID),
	}
}

func (v *VoteRegister) InsertVote(vote *abdrc.VoteMsg, quorumInfo QuorumInfo) (*drctypes.QuorumCert, error) {
	if vote == nil {
		return nil, ErrVoteIsNil
	}

	// Get hash of consensus structure
	bs, err := vote.LedgerCommitInfo.SigBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unicity seal: %w", err)
	}
	commitInfoHash := sha256.Sum256(bs)

	// has the author already voted in this round?
	if prevVoteHash, voted := v.authorToVote[vote.Author]; voted {
		// Check if vote has changed
		if commitInfoHash != prevVoteHash {
			// new equivocating vote, this is a security event
			return nil, fmt.Errorf("equivocating vote, previous %X, new %X", prevVoteHash, commitInfoHash)
		}
		return nil, fmt.Errorf("duplicate vote")
	}
	// Store vote from author
	v.authorToVote[vote.Author] = commitInfoHash
	// register commit hash
	// Create new entry if not present
	if _, present := v.hashToSignatures[commitInfoHash]; !present {
		v.hashToSignatures[commitInfoHash] = &ConsensusWithSignatures{
			commitInfo: vote.LedgerCommitInfo,
			voteInfo:   vote.VoteInfo,
			signatures: make(map[string]hex.Bytes),
		}
	}
	// Add signature from vote
	quorum := v.hashToSignatures[commitInfoHash]
	quorum.signatures[vote.Author] = vote.Signature
	// Check QC
	if uint64(len(quorum.signatures)) >= quorumInfo.GetQuorumThreshold() {
		qc := drctypes.NewQuorumCertificateFromVote(quorum.voteInfo, quorum.commitInfo, quorum.signatures)
		return qc, nil
	}
	// Vote registered, no QC could be formed
	return nil, nil
}

/*
InsertTimeoutVote returns non nil TC when quorum has been achieved.
Second return value is number of signatures in the TC.
*/
func (v *VoteRegister) InsertTimeoutVote(timeout *abdrc.TimeoutMsg, quorumInfo QuorumInfo) (*drctypes.TimeoutCert, uint64, error) {
	// Create partial timeout cert on first vote received
	if v.timeoutCert == nil {
		v.timeoutCert = &drctypes.TimeoutCert{
			Timeout:    timeout.Timeout,
			Signatures: make(map[string]*drctypes.TimeoutVote),
		}
	}
	// append signature
	if err := v.timeoutCert.Add(timeout.Author, timeout.Timeout, timeout.Signature); err != nil {
		return nil, 0, fmt.Errorf("failed to add vote to timeout certificate: %w", err)
	}

	// Check if TC can be formed
	sigCount := uint64(len(v.timeoutCert.Signatures))
	if sigCount >= quorumInfo.GetQuorumThreshold() {
		return v.timeoutCert, sigCount, nil
	}
	// No quorum yet, but also no error all is fine
	return nil, sigCount, nil
}

func (v *VoteRegister) Reset() {
	clear(v.hashToSignatures)
	clear(v.authorToVote)
	v.timeoutCert = nil
}
