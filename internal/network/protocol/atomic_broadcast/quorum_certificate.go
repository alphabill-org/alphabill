package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrTimeoutIsNil          = errors.New("timeout is nil")
	ErrVoteInfoIsNil         = errors.New("vote info is nil")
	ErrLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
	ErrQcIsMissingSignatures = errors.New("qc is missing signatures")
)

func NewQuorumCertificateFromVote(voteInfo *certificates.RootRoundInfo, commitInfo *certificates.CommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func NewQuorumCertificate(voteInfo *certificates.RootRoundInfo, commitHash []byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: &certificates.CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256), RootHash: commitHash},
		Signatures:       map[string][]byte{},
	}
}

func (x *QuorumCert) IsValid() error {
	// QC must have valid vote info
	if x.VoteInfo == nil {
		return ErrVoteInfoIsNil
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return err
	}
	// and must have valid ledger commit info
	if x.LedgerCommitInfo == nil {
		return ErrLedgerCommitInfoIsNil
	}
	// For root validator commit state id can be empty
	if len(x.LedgerCommitInfo.RootRoundInfoHash) < 1 {
		return certificates.ErrInvalidRootInfoHash
	}
	if len(x.Signatures) < 1 {
		return ErrQcIsMissingSignatures
	}
	return nil
}

func (x *QuorumCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	// check vote info hash
	hash := x.VoteInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(hash, x.LedgerCommitInfo.RootRoundInfoHash) {
		return fmt.Errorf("quorum certificate not valid: vote info hash verification failed")
	}
	// Check quorum, if not fail without checking signatures itself
	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("quorum certificate not valid: less than quorum %d/%d have signed",
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
			return fmt.Errorf("quorum certificate not valid: %w", err)
		}
	}
	return nil
}