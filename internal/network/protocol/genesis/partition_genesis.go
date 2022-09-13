package genesis

import (
	"bytes"
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrPartitionGenesisIsNil = errors.New("partition genesis is nil")
var ErrRootChainEncryptionKeyMissing = errors.New("root encryption public key is missing")
var ErrKeysAreMissing = errors.New("partition keys are missing")
var ErrKeyIsNil = errors.New("key is nil")
var ErrMissingRootValidators = errors.New("Missing root validators")

func (x *PartitionGenesis) FindRootPubKeyInfoById(id string) *PublicKeyInfo {
	// linear search for id
	for _, info := range x.RootValidators {
		if info.NodeIdentifier == id {
			return info
		}
	}
	return nil
}

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
	if len(x.RootValidators) < 1 {
		return ErrMissingRootValidators
	}
	for _, node := range x.RootValidators {
		err := node.IsValid()
		if err != nil {
			return errors.Wrap(err, "invalid root validator public key info")
		}
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
	// Make sure the public key information was added
	// todo root genesis: need to verify that public key info matches signatures in UC
	rootId := x.RootValidators[0].NodeIdentifier
	pubKeyInfo := x.FindRootPubKeyInfoById(string(rootId))
	if pubKeyInfo == nil {
		return errors.Errorf("Root validator id %v is missing", string(rootId))
	}
	if !bytes.Equal(pubKeyBytes, pubKeyInfo.SigningPublicKey) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, pubKeyInfo.SigningPublicKey)
	}
	sdrHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := x.Certificate.IsValid(verifier, hashAlgorithm, x.SystemDescriptionRecord.SystemIdentifier, sdrHash); err != nil {
		return err
	}
	return nil
}
