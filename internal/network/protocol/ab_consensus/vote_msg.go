package ab_consensus

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
)

func (x *VoteMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errSignerIsNil
	}
	// sanity check, make sure commit info round info hash is set
	if len(x.LedgerCommitInfo.RootRoundInfoHash) < 1 {
		return certificates.ErrInvalidRootInfoHash
	}
	signature, err := signer.SignBytes(x.LedgerCommitInfo.Bytes())
	if err != nil {
		return fmt.Errorf("vote sign failed, %w", err)
	}
	x.Signature = signature
	return nil
}

func (x *VoteMsg) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if x.VoteInfo == nil {
		return fmt.Errorf("invalid vote message, vote info is nil")
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("invalid vote message, %w", err)
	}
	// Verify hash of vote info
	hash := x.VoteInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(hash, x.LedgerCommitInfo.RootRoundInfoHash) {
		return fmt.Errorf("invalid vote message, vote info hash verification failed")
	}
	if len(x.Author) == 0 {
		return fmt.Errorf("invalid vote message, no author")
	}
	if err := x.HighQc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid vote message, %w", err)
	}
	// verify signature
	v, f := rootTrust[x.Author]
	if !f {
		return fmt.Errorf("failed to find public key for author %v", x.Author)
	}
	if err := v.VerifyBytes(x.Signature, x.LedgerCommitInfo.Bytes()); err != nil {
		return fmt.Errorf("vote message signature verification error, %w", err)
	}
	return nil
}
