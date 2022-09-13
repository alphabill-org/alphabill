package genesis

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

func (x *PublicKeyInfo) IsValid() error {
	if x == nil {
		return errors.New("PublicKeyInfo is nil")
	}
	if x.NodeIdentifier == "" {
		return ErrNodeIdentifierIsEmpty
	}
	if len(x.SigningPublicKey) == 0 {
		return ErrSigningPublicKeyIsInvalid
	}
	_, err := crypto.NewVerifierSecp256k1(x.SigningPublicKey)
	if err != nil {
		return err
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrEncryptionPublicKeyIsInvalid
	}
	_, err = crypto.NewVerifierSecp256k1(x.EncryptionPublicKey)
	if err != nil {
		return err
	}
	return nil
}
