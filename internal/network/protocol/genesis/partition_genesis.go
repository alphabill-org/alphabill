package genesis

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/certificates"
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

func (x *PartitionGenesis) IsValid(verifiers map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrPartitionGenesisIsNil
	}
	if len(verifiers) == 0 {
		return ErrMissingPubKeyInfo
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
	if x.SystemDescriptionRecord == nil {
		return ErrSystemDescriptionIsNil
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	if x.Certificate == nil {
		return certificates.ErrUnicityCertificateIsNil
	}
	sdrHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	// validate all signatures against known root keys
	if err := x.Certificate.IsValid(verifiers, hashAlgorithm, x.SystemDescriptionRecord.SystemIdentifier, sdrHash); err != nil {
		return err
	}
	// UC Seal must be signed by all validators
	if len(x.RootValidators) != len(x.Certificate.UnicitySeal.Signatures) {
		return errors.New("Unicity Certificate is not signed by all root validators")
	}
	return nil
}
