package distributed

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
)

type (
	QuorumInfo interface {
		GetQuorumThreshold() uint32
	}

	ConsensusWithSignatures struct {
		voteInfo   *certificates.RootRoundInfo
		commitInfo *certificates.CommitInfo
		signatures map[string][]byte
	}

	VoteRegister struct {
		// Hash of ConsensusInfo to signatures/votes for it
		hashToSignatures map[string]*ConsensusWithSignatures
		// Tracks all timout votes for this round
		// if 2f+1 or threshold votes, then TC is formed
		timeoutCert *ab_consensus.TimeoutCert
		// Helper, to avoid duplicate votes
		authorToVote map[string][]byte
	}
)

var ErrVoteIsNil = errors.New("vote is nil")

func NewVoteRegister() *VoteRegister {
	return &VoteRegister{
		hashToSignatures: make(map[string]*ConsensusWithSignatures),
		timeoutCert:      nil,
		authorToVote:     make(map[string][]byte),
	}
}

func (v *VoteRegister) InsertVote(vote *ab_consensus.VoteMsg, quorumInfo QuorumInfo) (*ab_consensus.QuorumCert, error) {
	if vote == nil {
		return nil, ErrVoteIsNil
	}

	// Get hash of consensus structure
	commitInfoHash := vote.LedgerCommitInfo.Hash(crypto.SHA256)

	// has the author already voted in this round?
	if prevVoteHash, voted := v.authorToVote[vote.Author]; voted {
		// Check if vote has changed
		if !bytes.Equal(commitInfoHash, prevVoteHash) {
			// new equivocating vote, this is a security event
			return nil, fmt.Errorf("equivocating vote, previous %X, new %X", prevVoteHash, commitInfoHash)
		}
		return nil, fmt.Errorf("duplicate vote")
	}
	// Store vote from author
	v.authorToVote[vote.Author] = commitInfoHash
	// register commit hash
	// Create new entry if not present
	if _, present := v.hashToSignatures[string(commitInfoHash)]; !present {
		v.hashToSignatures[string(commitInfoHash)] = &ConsensusWithSignatures{
			commitInfo: vote.LedgerCommitInfo,
			voteInfo:   vote.VoteInfo,
			signatures: make(map[string][]byte),
		}
	}
	// Add signature from vote
	quorum := v.hashToSignatures[string(commitInfoHash)]
	quorum.signatures[vote.Author] = vote.Signature
	// Check QC
	if uint32(len(quorum.signatures)) >= quorumInfo.GetQuorumThreshold() {
		qc := ab_consensus.NewQuorumCertificateFromVote(quorum.voteInfo, quorum.commitInfo, quorum.signatures)
		return qc, nil
	}
	// Vote registered, no QC could be formed
	return nil, nil
}

func (v *VoteRegister) InsertTimeoutVote(timeout *ab_consensus.TimeoutMsg, quorumInfo QuorumInfo) (*ab_consensus.TimeoutCert, error) {
	// Create partial timeout cert on first vote received
	if v.timeoutCert == nil {
		v.timeoutCert = &ab_consensus.TimeoutCert{
			Timeout:    timeout.Timeout,
			Signatures: make(map[string]*ab_consensus.TimeoutVote),
		}
	}
	// append signature
	if err := v.timeoutCert.Add(timeout.Author, timeout.Timeout, timeout.Signature); err != nil {
		return nil, fmt.Errorf("timeout cert add vote failed, %w", err)
	}
	// Check if TC can be formed
	if uint32(len(v.timeoutCert.GetSignatures())) >= quorumInfo.GetQuorumThreshold() {
		return v.timeoutCert, nil
	}
	// No quorum yet, but also no error all is fine
	return nil, nil
}

func (v *VoteRegister) Reset() {
	v.hashToSignatures = make(map[string]*ConsensusWithSignatures)
	v.timeoutCert = nil
	v.authorToVote = make(map[string][]byte)
}
