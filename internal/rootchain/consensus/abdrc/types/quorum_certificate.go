package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"
	"sort"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	errVoteInfoIsNil         = errors.New("vote info is nil")
	errLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
	errInvalidRoundInfoHash  = errors.New("round info has is missing")
)

type QuorumCert struct {
	_                struct{}           `cbor:",toarray"`
	VoteInfo         *RoundInfo         `json:"vote_info,omitempty"`          // Consensus data
	LedgerCommitInfo *types.UnicitySeal `json:"ledger_commit_info,omitempty"` // Commit info
	Signatures       map[string][]byte  `json:"signatures,omitempty"`         // Node identifier to signature map (NB! aggregated signature schema in spec)
}

func NewQuorumCertificateFromVote(voteInfo *RoundInfo, commitInfo *types.UnicitySeal, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func NewQuorumCertificate(voteInfo *RoundInfo, commitHash []byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256), Hash: commitHash},
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
	if x.VoteInfo == nil {
		return errVoteInfoIsNil
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("invalid vote info: %w", err)
	}

	// and must have valid ledger commit info
	if x.LedgerCommitInfo == nil {
		return errLedgerCommitInfoIsNil
	}
	// todo: should call x.LedgerCommitInfo.IsValid but that requires some refactoring
	// not to require trustbase parameter?
	// For root validator commit state id can be empty
	if len(x.LedgerCommitInfo.RootInternalInfo) < 1 {
		return errInvalidRoundInfoHash
	}

	return nil
}

func (x *QuorumCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid quorum certificate: %w", err)
	}
	// check vote info hash
	if !bytes.Equal(x.VoteInfo.Hash(gocrypto.SHA256), x.LedgerCommitInfo.RootInternalInfo) {
		return fmt.Errorf("vote info hash verification failed")
	}
	/* todo: call LedgerCommitInfo.Verify but first refactor it so that it takes quorum param?
	if err := x.LedgerCommitInfo.Verify(rootTrust); err != nil {
		return fmt.Errorf("invalid commit info: %w", err)
	}*/
	// Check quorum, if not fail without checking signatures
	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("quorum requires %d signatures but certificate has %d", quorum, len(x.Signatures))
	}
	// Verify all signatures
	for author, sig := range x.Signatures {
		ver, f := rootTrust[author]
		if !f {
			return fmt.Errorf("signer %q is not part of trustbase", author)
		}
		if err := ver.VerifyBytes(sig, x.LedgerCommitInfo.Bytes()); err != nil {
			return fmt.Errorf("signer %q signature is not valid: %w", author, err)
		}
	}
	return nil
}

func (x *QuorumCert) AddSignersToHasher(hasher hash.Hash) {
	if x != nil {
		// From QC signers (in the alphabetical order of signer ID!) must be included
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
