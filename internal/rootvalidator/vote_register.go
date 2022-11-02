package rootvalidator

import (
	"bytes"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
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

func NewVoteRegister() *VoteRegister {
	return &VoteRegister{
		HashToSignatures: make(map[string]*ConsensusWithSignatures),
		TimeoutCert:      nil,
		AuthorToVote:     make(map[peer.ID]*atomic_broadcast.VoteMsg),
	}
}

func (v *VoteRegister) InsertVote(vote *atomic_broadcast.VoteMsg, verifier *RootNodeVerifier) (*atomic_broadcast.QuorumCert, *atomic_broadcast.TimeoutCert) {
	// Get hash of consensus structure
	commitInfoHash := vote.LedgerCommitInfo.Hash(crypto.SHA256)

	// has the author already voted in this round?
	prevVote, voted := v.AuthorToVote[peer.ID(vote.Author)]
	if voted {
		// Check if vote has changed
		if !bytes.Equal(commitInfoHash, prevVote.LedgerCommitInfo.Hash(crypto.SHA256)) {
			// new equivocating vote, this is a security event
			logger.Warning("Received equivocating vote from %v, round %v",
				vote.Author, vote.VoteInfo.RootRound)
			// Todo: Add return error, how do we register this event
			return nil, nil
		}
		// Is new vote for timeout
		newTimeoutVote := vote.IsTimeout() && !prevVote.IsTimeout()
		if !newTimeoutVote {
			logger.Warning("Received duplicate vote from %v, round %v",
				vote.Author, vote.VoteInfo.RootRound)
			return nil, nil
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
	if verifier.GetQuorumThreshold() >= uint32(len(quorum.signatures)) {
		qc := atomic_broadcast.NewQuorumCertificate(quorum.voteInfo, quorum.commitInfo, quorum.signatures)
		return qc, nil
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
		if verifier.GetQuorumThreshold() >= uint32(len(v.TimeoutCert.GetSignatures())) {
			return nil, v.TimeoutCert
		}
	}
	// Vote registered, no QC or TC could be formed
	return nil, nil
}

func (v *VoteRegister) Reset() {
	v.HashToSignatures = make(map[string]*ConsensusWithSignatures)
	v.TimeoutCert = nil
	v.AuthorToVote = make(map[peer.ID]*atomic_broadcast.VoteMsg)
}
