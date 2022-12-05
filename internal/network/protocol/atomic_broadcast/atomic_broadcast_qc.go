package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"hash"
)

var (
	ErrTimeoutIsNil          = errors.New("timeout is nil")
	ErrVoteInfoIsNil         = errors.New("vote info is nil")
	ErrLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
	ErrMissingSignatures     = errors.New("qc is missing signatures")
)

func NewQuorumCertificate(voteInfo *VoteInfo, commitInfo *LedgerCommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
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
	if err := x.LedgerCommitInfo.IsValid(); err != nil {
		return err
	}
	if len(x.Signatures) < 1 {
		return ErrMissingSignatures
	}
	return nil
}

func (x *QuorumCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	// check vote info hash
	hasher := gocrypto.SHA256.New()
	x.VoteInfo.AddToHasher(hasher)
	if !bytes.Equal(hasher.Sum(nil), x.LedgerCommitInfo.VoteInfoHash) {
		return fmt.Errorf("quorum certificate not valid: vote info hash verification failed")
	}
	hasher.Reset()
	hasher.Write(x.LedgerCommitInfo.Bytes())
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
		err := ver.VerifyHash(hasher.Sum(nil), sig)
		if err != nil {
			return fmt.Errorf("quorum certificate not valid: %w", err)
		}
	}
	return nil
}

func (x *QuorumCert) AddToHasher(hasher hash.Hash) {
	x.VoteInfo.AddToHasher(hasher)
	hasher.Write(x.LedgerCommitInfo.Bytes())
	// Add all signatures
	for author, sig := range x.Signatures {
		hasher.Write([]byte(author))
		hasher.Write(sig)
	}
}
