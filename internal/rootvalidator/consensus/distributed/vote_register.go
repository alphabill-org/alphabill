package distributed

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	QuorumInfo interface {
		GetQuorumThreshold() uint32
	}

	ConsensusWithSignatures struct {
		voteInfo   *atomic_broadcast.VoteInfo
		commitInfo *atomic_broadcast.LedgerCommitInfo
		signatures map[string][]byte
	}

	VoteRegister struct {
		// Hash of ConsensusInfo to signatures/votes for it
		hashToSignatures map[string]*ConsensusWithSignatures
		// Tracks all timout votes for this round
		// if 2f+1 or threshold votes, then TC is formed
		timeoutCert *atomic_broadcast.TimeoutCert
		// Helper, to avoid duplicate votes
		authorToVote map[peer.ID]*atomic_broadcast.VoteMsg
	}
)

var ErrVoteIsNil = errors.New("vote is nil")

func NewVoteRegister() *VoteRegister {
	return &VoteRegister{
		hashToSignatures: make(map[string]*ConsensusWithSignatures),
		timeoutCert:      nil,
		authorToVote:     make(map[peer.ID]*atomic_broadcast.VoteMsg),
	}
}

func (v *VoteRegister) InsertVote(vote *atomic_broadcast.VoteMsg, quorumInfo QuorumInfo) (*atomic_broadcast.QuorumCert, error) {
	if vote == nil {
		return nil, ErrVoteIsNil
	}

	// Get hash of consensus structure
	commitInfoHash := vote.LedgerCommitInfo.Hash(crypto.SHA256)

	// has the author already voted in this round?
	prevVote, voted := v.authorToVote[peer.ID(vote.Author)]
	if voted {
		prevVoteHash := prevVote.LedgerCommitInfo.Hash(crypto.SHA256)
		// Check if vote has changed
		if !bytes.Equal(commitInfoHash, prevVoteHash) {
			// new equivocating vote, this is a security event
			return nil, fmt.Errorf("equivocating vote, previous %X, new %X", prevVoteHash, commitInfoHash)
		}
		return nil, fmt.Errorf("duplicate vote")
	}
	// Store vote from author
	v.authorToVote[peer.ID(vote.Author)] = vote
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
		qc := atomic_broadcast.NewQuorumCertificate(quorum.voteInfo, quorum.commitInfo, quorum.signatures)
		return qc, nil
	}
	// Vote registered, no QC could be formed
	return nil, nil
}

func (v *VoteRegister) InsertTimeoutVote(timeout *atomic_broadcast.TimeoutMsg, quorumInfo QuorumInfo) (*atomic_broadcast.TimeoutCert, error) {
	// Create partial timeout cert on first vote received
	if v.timeoutCert == nil {
		v.timeoutCert = &atomic_broadcast.TimeoutCert{
			Timeout:    timeout.Timeout,
			Signatures: make(map[string]*atomic_broadcast.TimeoutVote),
		}
	}
	// append signature
	v.timeoutCert.Add(timeout.Author, timeout.Timeout, timeout.Signature)
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
	v.authorToVote = make(map[peer.ID]*atomic_broadcast.VoteMsg)
}
