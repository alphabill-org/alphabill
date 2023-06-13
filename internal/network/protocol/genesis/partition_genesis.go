package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	ErrPartitionGenesisIsNil            = errors.New("partition genesis is nil")
	ErrKeysAreMissing                   = errors.New("partition keys are missing")
	ErrMissingRootValidators            = errors.New("missing root nodes")
	ErrPartitionUnicityCertificateIsNil = errors.New("partition unicity certificate is nil")
)

type PartitionGenesis struct {
	_                       struct{}                  `cbor:",toarray"`
	SystemDescriptionRecord *SystemDescriptionRecord  `json:"system_description_record,omitempty"`
	Certificate             *types.UnicityCertificate `json:"certificate,omitempty"`
	RootValidators          []*PublicKeyInfo          `json:"root_validators,omitempty"`
	Keys                    []*PublicKeyInfo          `json:"keys,omitempty"`
	Params                  []byte                    `json:"params,omitempty"`
}

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
		return ErrVerifiersEmpty
	}
	if len(x.Keys) < 1 {
		return ErrKeysAreMissing
	}
	if len(x.RootValidators) < 1 {
		return ErrMissingRootValidators
	}
	// check that root validators are valid and
	// make sure it is a list of unique node ids and keys
	if err := ValidatorInfoUnique(x.RootValidators); err != nil {
		return fmt.Errorf("root node list validation failed, %w", err)
	}
	// check partition validator public info is valid, and
	// it is a list of unique node ids and keys
	if err := ValidatorInfoUnique(x.Keys); err != nil {
		return fmt.Errorf("partition keys validation failed, %w", err)
	}

	if x.SystemDescriptionRecord == nil {
		return ErrSystemDescriptionIsNil
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return fmt.Errorf("invalid system decsrition record, %w", err)
	}
	if x.Certificate == nil {
		return ErrPartitionUnicityCertificateIsNil
	}
	sdrHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	// validate all signatures against known root keys
	if err := x.Certificate.IsValid(verifiers, hashAlgorithm, x.SystemDescriptionRecord.SystemIdentifier, sdrHash); err != nil {
		return fmt.Errorf("invalid unicity certificate, %w", err)
	}
	// UC Seal must be signed by all validators
	if len(x.RootValidators) != len(x.Certificate.UnicitySeal.Signatures) {
		return fmt.Errorf("unicity Certificate is not signed by all root nodes")
	}
	return nil
}
