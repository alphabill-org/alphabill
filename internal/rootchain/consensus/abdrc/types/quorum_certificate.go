package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"
	"sort"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	errVoteInfoIsNil         = errors.New("vote info is nil")
	errLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
	errQcIsMissingSignatures = errors.New("qc is missing signatures")
)

type QuorumCert struct {
	_                struct{}          `cbor:",toarray"`
	VoteInfo         *RoundInfo        `json:"vote_info,omitempty"`          // Consensus data
	LedgerCommitInfo *CommitInfo       `json:"ledger_commit_info,omitempty"` // Commit info
	Signatures       map[string][]byte `json:"signatures,omitempty"`         // Node identifier to signature map (NB! aggregated signature schema in spec)
}

func NewQuorumCertificateFromVote(voteInfo *RoundInfo, commitInfo *CommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func NewQuorumCertificate(voteInfo *RoundInfo, commitHash []byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: &CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256), RootHash: commitHash},
		Signatures:       map[string][]byte{},
	}
}

func (x *QuorumCert) GetRound() uint64 {
	if x != nil {
		return x.VoteInfo.GetRound()
	}
	return 0
}

func (x *QuorumCert) GetParentRound() uint64 {
	if x != nil {
		return x.VoteInfo.GetParentRound()
	}
	return 0
}

func (x *QuorumCert) IsValid() error {
	// QC must have valid vote info
	if x.VoteInfo == nil {
		return errVoteInfoIsNil
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("vote info not valid, %w", err)
	}
	// and must have valid ledger commit info
	if x.LedgerCommitInfo == nil {
		return errLedgerCommitInfoIsNil
	}
	// For root validator commit state id can be empty
	if len(x.LedgerCommitInfo.RootRoundInfoHash) < 1 {
		return errInvalidRoundInfoHash
	}
	if len(x.Signatures) < 1 {
		return errQcIsMissingSignatures
	}
	return nil
}

func (x *QuorumCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("quorum certificate validation failed, %w", err)
	}
	// check vote info hash
	h := x.VoteInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(h, x.LedgerCommitInfo.RootRoundInfoHash) {
		return fmt.Errorf("vote info hash verification failed")
	}
	// Check quorum, if not fail without checking signatures itself
	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("certificate has less signatures %d than required by quorum %d",
			len(x.Signatures), quorum)
	}
	// Verify all signatures
	for author, sig := range x.Signatures {
		ver, f := rootTrust[author]
		if !f {
			return fmt.Errorf("failed to find public key for author %v", author)
		}
		err := ver.VerifyBytes(sig, x.LedgerCommitInfo.Bytes())
		if err != nil {
			return fmt.Errorf("node %v signature is not valid: %w", author, err)
		}
	}
	return nil
}

func (x *QuorumCert) AddSignaturesToHasher(hasher hash.Hash) {
	if x != nil {
		// From QC signatures (in the alphabetical order of signer ID!) must be included
		signatures := x.Signatures
		authors := make([]string, 0, len(signatures))
		for k := range signatures {
			authors = append(authors, k)
		}
		sort.Strings(authors)
		for _, author := range authors {
			hasher.Write(signatures[author])
		}
	}
}
