package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
	ErrVerifiersEmpty              = errors.New("verifier list is empty")
)

type GenesisPartitionRecord struct {
	_                       struct{}                       `cbor:",toarray"`
	Nodes                   []*PartitionNode               `json:"nodes,omitempty"`
	Certificate             *types.UnicityCertificate      `json:"certificate,omitempty"`
	SystemDescriptionRecord *types.SystemDescriptionRecord `json:"system_description_record,omitempty"`
}

func (x *GenesisPartitionRecord) GetSystemDescriptionRecord() *types.SystemDescriptionRecord {
	if x == nil {
		return nil
	}
	return x.SystemDescriptionRecord
}

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
	if err := x.Certificate.Verify(verifiers, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("unicity certificate verify error: %w", err)
	}
	return nil
}
