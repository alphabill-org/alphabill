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
	// check that root validator public info is valid
	for _, node := range x.RootValidators {
		err := node.IsValid()
		if err != nil {
			return errors.Wrap(err, "invalid root validator public key info")
		}
	}
	// make sure it is a list of unique node ids and keys
	err := ValidatorInfoUnique(x.RootValidators)
	if err != nil {
		return errors.Wrap(err, "invalid root validator list")
	}
	// check partition validator public info is valid
	for _, keyInfo := range x.Keys {
		err := keyInfo.IsValid()
		if err != nil {
			return errors.Wrap(err, "invalid partition node validator public key info")
		}
	}
	// make sure it is a list of unique node ids and keys
	err = ValidatorInfoUnique(x.Keys)
	if err != nil {
		return errors.Wrap(err, "invalid partition validator list")
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
