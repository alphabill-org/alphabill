package genesis

import (
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

var (
	ErrPartitionIsNil  = errors.New("partition is nil")
	ErrNodesAreMissing = errors.New("nodes are missing")
)

func (x *GenesisPartitionRecord) IsValid(verifier crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrPartitionIsNil
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
	if err := x.Certificate.IsValid(verifier, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	return nil
}

func (x *GenesisPartitionRecord) GetSystemIdentifierString() string {
	return string(x.SystemDescriptionRecord.SystemIdentifier)
}
