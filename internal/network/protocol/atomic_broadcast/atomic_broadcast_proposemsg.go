package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
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

func (x *ProposalMsg) Verify(p PartitionVerifier, v AtomicVerifier) error {
	if v == nil {
		return ErrVerifierIsNil
	}
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	err = v.VerifySignature(hash, x.Signature, peer.ID(x.Block.Author))
	if err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// Check certificates
	if err := x.Block.Qc.Verify(v); err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// If there is a high commit QC certificate (is this optional at all)
	if x.HighCommitQc == nil {
		return errors.New("proposal is missing commit certificate")
	}
	if err := x.HighCommitQc.Verify(v); err != nil {
		return aberrors.Wrap(err, "proposal verification failed")
	}
	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err := x.LastRoundTc.Verify(v); err != nil {
			return aberrors.Wrap(err, "proposal verification failed")
		}
	}
	if err := x.Block.IsValid(p, v); err != nil {
		return err
	}
	return nil
}
