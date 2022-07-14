package certificates

import (
	"bytes"
	"hash"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrUnicitySealIsNil             = errors.New("unicity seal is nil")
	ErrSignerIsNil                  = errors.New("signer is nil")
	ErrUnicitySealVerifierIsNil     = errors.New("unicity seal verifier is nil")
	ErrVerifierIsNil                = errors.New("verifier is nil")
	ErrUnicitySealHashIsNil         = errors.New("hash is nil")
	ErrUnicitySealPreviousHashIsNil = errors.New("previous hash is nil")
	ErrInvalidBlockNumber           = errors.New("invalid block number")
	ErrUnicitySealSignatureIsNil    = errors.New("signature is nil")
)

func (x *UnicitySeal) IsValid(verifier crypto.Verifier) error {
	if x == nil {
		return ErrUnicitySealIsNil
	}
	if verifier == nil {
		return ErrUnicitySealVerifierIsNil
	}
	if x.Hash == nil {
		return ErrUnicitySealHashIsNil
	}
	if x.PreviousHash == nil {
		return ErrUnicitySealPreviousHashIsNil
	}
	if x.RootChainRoundNumber < 1 {
		return ErrInvalidBlockNumber
	}
	if x.Signature == nil {
		return ErrUnicitySealSignatureIsNil
	}
	return x.Verify(verifier)
}

func (x *UnicitySeal) Sign(signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *UnicitySeal) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.RootChainRoundNumber))
	b.Write(x.PreviousHash)
	b.Write(x.Hash)
	return b.Bytes()
}

func (x *UnicitySeal) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *UnicitySeal) Verify(v crypto.Verifier) error {
	if v == nil {
		return ErrVerifierIsNil
	}
	err := v.VerifyBytes(x.Signature, x.Bytes())
	if err != nil {
		return errors.Wrap(err, "invalid unicity seal signature")
	}
	return err
}
