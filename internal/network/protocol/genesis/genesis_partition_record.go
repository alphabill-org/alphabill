package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
	ErrVerifiersEmpty              = errors.New("verifier list is empty")
)

func (x *GenesisPartitionRecord) IsValid(verifiers map[string]crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrGenesisPartitionRecordIsNil
	}
	if len(verifiers) == 0 {
		return ErrVerifiersEmpty
	}
	if len(x.Nodes) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return fmt.Errorf("system decrition validation failed, %w", err)
	}
	systemIdentifier := x.SystemDescriptionRecord.SystemIdentifier
	systemDescriptionHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := nodesUnique(x.Nodes); err != nil {
		return fmt.Errorf("partition nodes validation failed, %w", err)
	}
	if err := x.Certificate.IsValid(verifiers, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("unicity certificate validation failed, %w", err)
	}
	return nil
}

func (x *GenesisPartitionRecord) GetSystemIdentifierString() p.SystemIdentifier {
	return p.SystemIdentifier(x.SystemDescriptionRecord.SystemIdentifier)
}
