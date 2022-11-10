package atomic_broadcast

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ErrSignerIsNil   = "signer is nil"
	ErrVerifierIsNil = "verifier is nil"
)

func (x *ProposalMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errors.New(ErrSignerIsNil)
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

func (x *ProposalMsg) Verify(p PartitionStore, v AtomicVerifier) error {
	if v == nil {
		return errors.New(ErrVerifierIsNil)
	}
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	err = v.VerifySignature(hash, x.Signature, peer.ID(x.Block.Author))
	if err != nil {
		return errors.Wrap(err, "proposal verification failed")
	}
	// Check certificates
	if err := x.Block.Qc.Verify(v); err != nil {
		return errors.Wrap(err, "proposal verification failed")
	}
	// If there is a high commit QC certificate (is this optional at all)
	if x.HighCommitQc == nil {
		return errors.New("proposal is missing commit certificate")
	}
	if err := x.HighCommitQc.Verify(v); err != nil {
		return errors.Wrap(err, "proposal verification failed")
	}
	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err := x.LastRoundTc.Verify(v); err != nil {
			return errors.Wrap(err, "proposal verification failed")
		}
	}
	if err := x.Block.IsValid(p, v); err != nil {
		return err
	}
	return nil
}
