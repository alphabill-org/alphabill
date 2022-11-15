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
		HashToSignatures map[string]*ConsensusWithSignatures
		// Tracks all timout votes for this round
		// if 2f+1 or threshold votes, then TC is formed
		TimeoutCert *atomic_broadcast.TimeoutCert
		// Helper, to avoid duplicate votes
		AuthorToVote map[peer.ID]*atomic_broadcast.VoteMsg
	}
)

var ErrVoteIsNil = errors.New("vote is nil")

func NewVoteRegister() *VoteRegister {
	return &VoteRegister{
		HashToSignatures: make(map[string]*ConsensusWithSignatures),
		TimeoutCert:      nil,
		AuthorToVote:     make(map[peer.ID]*atomic_broadcast.VoteMsg),
	}
}

func (v *VoteRegister) InsertVote(vote *atomic_broadcast.VoteMsg, quorumInfo QuorumInfo) (*atomic_broadcast.QuorumCert, *atomic_broadcast.TimeoutCert, error) {
	if vote == nil {
		return nil, nil, ErrVoteIsNil
	}

	// Get hash of consensus structure
	commitInfoHash := vote.LedgerCommitInfo.Hash(crypto.SHA256)

	// has the author already voted in this round?
	prevVote, voted := v.AuthorToVote[peer.ID(vote.Author)]
	if voted {
		prevVoteHash := prevVote.LedgerCommitInfo.Hash(crypto.SHA256)
		// Check if vote has changed
		if !bytes.Equal(commitInfoHash, prevVoteHash) {
			// new equivocating vote, this is a security event
			return nil, nil, fmt.Errorf("equivocating vote, previous %X, new %X", prevVoteHash, commitInfoHash)
		}
		// Is new vote for timeout
		newTimeoutVote := vote.IsTimeout() && !prevVote.IsTimeout()
		if !newTimeoutVote {
			return nil, nil, fmt.Errorf("duplicate vote")
		}
		// Author voted timeout, proceed
		logger.Info("Received timout vote from %v, round %v",
			vote.Author, vote.VoteInfo.RootRound)
	}
	// Store vote from author
	v.AuthorToVote[peer.ID(vote.Author)] = vote
	// register commit hash
	// Create new entry if not present
	if _, present := v.HashToSignatures[string(commitInfoHash)]; !present {
		v.HashToSignatures[string(commitInfoHash)] = &ConsensusWithSignatures{
			commitInfo: vote.LedgerCommitInfo,
			voteInfo:   vote.VoteInfo,
			signatures: make(map[string][]byte),
		}
	}
	// Add signature from vote
	quorum := v.HashToSignatures[string(commitInfoHash)]
	quorum.signatures[vote.Author] = vote.Signature
	// Check QC
	if uint32(len(quorum.signatures)) >= quorumInfo.GetQuorumThreshold() {
		qc := atomic_broadcast.NewQuorumCertificate(quorum.voteInfo, quorum.commitInfo, quorum.signatures)
		return qc, nil, nil
	}
	// Check TC
	if vote.IsTimeout() {
		// Create partial timeout cert on first vote received
		if v.TimeoutCert == nil {
			v.TimeoutCert = atomic_broadcast.NewPartialTimeoutCertificate(vote)
		}
		// append signature
		v.TimeoutCert.AddSignature(vote.Author, vote.TimeoutSignature)
		// Check if TC can be formed
		if uint32(len(v.TimeoutCert.GetSignatures())) >= quorumInfo.GetQuorumThreshold() {
			return nil, v.TimeoutCert, nil
		}
	}
	// Vote registered, no QC or TC could be formed
	return nil, nil, nil
}

func (v *VoteRegister) Reset() {
	v.HashToSignatures = make(map[string]*ConsensusWithSignatures)
	v.TimeoutCert = nil
	v.AuthorToVote = make(map[peer.ID]*atomic_broadcast.VoteMsg)
}
