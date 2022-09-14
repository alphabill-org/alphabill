package genesis

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/errors"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
)

func (x *GenesisPartitionRecord) IsValid(verifiers map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrGenesisPartitionRecordIsNil
	}
	if len(verifiers) == 0 {
		return ErrVerifierIsNil
	}
	if len(x.Nodes) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	systemIdentifier := x.SystemDescriptionRecord.SystemIdentifier
	systemDescriptionHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := nodesUnique(x.Nodes); err != nil {
		return err
	}
	if err := x.Certificate.IsValid(verifiers, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	return nil
}

func (x *GenesisPartitionRecord) GetSystemIdentifierString() string {
	return string(x.SystemDescriptionRecord.SystemIdentifier)
}
