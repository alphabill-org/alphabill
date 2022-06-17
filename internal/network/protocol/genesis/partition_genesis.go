package genesis

import (
	"bytes"
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrPartitionGenesisIsNil = errors.New("partition genesis is nil")
var ErrRootChainEncryptionKeyMissing = errors.New("root encryption public key is missing")
var ErrKeysAreMissing = errors.New("partition keys are missing")
var ErrKeyIsNil = errors.New("key is nil")

func (x *PartitionGenesis) IsValid(verifier crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrPartitionGenesisIsNil
	}
	if verifier == nil {
		return ErrVerifierIsNil
	}
	if len(x.Keys) < 1 {
		return ErrKeysAreMissing
	}
	if len(x.EncryptionKey) < 1 {
		return ErrRootChainEncryptionKeyMissing
	}
	_, err := crypto.NewVerifierSecp256k1(x.EncryptionKey)
	if err != nil {
		return err
	}
	for _, keyInfo := range x.Keys {
		if keyInfo == nil {
			return ErrKeyIsNil
		}
		if keyInfo.NodeIdentifier == "" {
			return ErrNodeIdentifierIsEmpty
		}
		if len(keyInfo.SigningPublicKey) == 0 {
			return ErrSigningPublicKeyIsInvalid
		}
		_, err := crypto.NewVerifierSecp256k1(keyInfo.SigningPublicKey)
		if err != nil {
			return err
		}
		if len(keyInfo.EncryptionPublicKey) == 0 {
			return ErrEncryptionPublicKeyIsInvalid
		}
		_, err = crypto.NewVerifierSecp256k1(keyInfo.EncryptionPublicKey)
		if err != nil {
			return err
		}
	}

	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(pubKeyBytes, x.TrustBase) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, x.TrustBase)
	}
	sdrHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := x.Certificate.IsValid(verifier, hashAlgorithm, x.SystemDescriptionRecord.SystemIdentifier, sdrHash); err != nil {
		return err
	}
	return nil
}
