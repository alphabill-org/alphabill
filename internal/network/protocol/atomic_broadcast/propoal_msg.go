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

func (x *ProposalMsg) IsValid() error {
	if x.Block == nil {
		return fmt.Errorf("proposal msg not valid, block is nil")
	}
	if err := x.Block.IsValid(); err != nil {
		return fmt.Errorf("proposal msg not valid, block error: %w", err)
	}
	return nil
}

func (x *ProposalMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	if err := x.Block.IsValid(); err != nil {
		return err
	}
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	// Sign block hash
	signature, err := signer.SignHash(hash)
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *ProposalMsg) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return fmt.Errorf("proposal msg, unexpected hash error %w", err)
	}
	// Find author public key
	v, f := rootTrust[x.Block.Author]
	if !f {
		return fmt.Errorf("proposal msg error, failed to find public key for root validator %v", x.Block.Author)
	}
	if err := v.VerifyHash(x.Signature, hash); err != nil {
		return aberrors.Wrap(err, "proposal msg signature verification failed")
	}
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("proposal msg not valid: %w", err)
	}
	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err := x.LastRoundTc.Verify(quorum, rootTrust); err != nil {
			return aberrors.Wrap(err, "proposal msg tc verification failed")
		}
	}
	if err := x.Block.Verify(quorum, rootTrust); err != nil {
		return aberrors.Wrap(err, "proposal msg block verification failed")
	}
	return nil
}