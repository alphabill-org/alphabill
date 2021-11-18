package crypto

import (
	"crypto/ed25519"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain/canonicalizer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
)

type (
	// deprecated
	verifierEd25519 struct {
		ecPubKey ed25519.PublicKey
	}
)

// NewEd25519Verifier creates new verifier from existing Ed25519 public key
// deprecated
func NewEd25519Verifier(pubKey []byte) Verifier {
	return &verifierEd25519{ecPubKey: pubKey}
}

// VerifyBytes verifies using the public key of the verifier
// deprecated
func (v *verifierEd25519) VerifyBytes(sig []byte, data []byte) error {
	if sig == nil || data == nil {
		return errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	if ed25519.Verify(v.ecPubKey, data, sig) {
		return nil
	}
	return errors.Wrap(errors.ErrVerificationFailed, "signature verify failed")
}

// VerifyObject verifies the signature of canonicalizable object with public key.
// deprecated
func (v *verifierEd25519) VerifyObject(sig []byte, obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) error {
	data, err := canonicalizer.Canonicalize(obj, opts...)
	if err != nil {
		return errors.Wrap(err, "could not canonicalize the object")
	}
	return v.VerifyBytes(sig, data)
}

// deprecated
func (v *verifierEd25519) MarshalPublicKey() ([]byte, error) {
	if v == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return v.ecPubKey, nil
}
