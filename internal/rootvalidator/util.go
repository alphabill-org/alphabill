package rootvalidator

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrSignerIsNil = errors.New("signer is nil")

func GetPublicKeyAndVerifier(signer crypto.Signer) ([]byte, crypto.Verifier, error) {
	if signer == nil {
		return nil, nil, ErrSignerIsNil
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}
	return pubKey, verifier, nil
}
