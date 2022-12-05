package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
)

var (
	ErrSignerIsNil   = errors.New("signer is nil")
	ErrVerifierIsNil = errors.New("verifier is nil")
)

func (x *ProposalMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	// Set block id
	x.Block.Id = hash
	// Sign block hash
	signature, err := signer.SignHash(hash)
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *ProposalMsg) Verify(p PartitionStore, quorum uint32, rootTrust map[string]crypto.Verifier) error {
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	v, f := rootTrust[x.Block.Author]
	if !f {
		return fmt.Errorf("failed to find public key for root validator %v", x.Block.Author)
	}
	err = v.VerifyHash(hash, x.Signature)
	if err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// Check certificates
	if err := x.Block.Qc.Verify(quorum, rootTrust); err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// If there is a high commit QC certificate (is this optional at all)
	if x.HighCommitQc == nil {
		return errors.New("proposal is missing commit certificate")
	}
	if err := x.HighCommitQc.Verify(quorum, rootTrust); err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err := x.LastRoundTc.Verify(quorum, rootTrust); err != nil {
			return aberrors.Wrap(err, "proposal verification failed")
		}
	}
	if err := x.Block.IsValid(p, quorum, rootTrust); err != nil {
		return err
	}
	return nil
}
