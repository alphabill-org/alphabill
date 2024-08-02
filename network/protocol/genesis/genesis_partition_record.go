package genesis

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
	ErrTrustBaseIsNil              = errors.New("trust base is nil")
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

func (x *GenesisPartitionRecord) IsValid(trustBase types.RootTrustBase, hashAlgorithm crypto.Hash) error {
	if x == nil {
		return ErrGenesisPartitionRecordIsNil
	}
	if trustBase == nil {
		return ErrTrustBaseIsNil
	}
	if len(x.Nodes) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return fmt.Errorf("invalid system description record: %w", err)
	}
	if err := nodesUnique(x.Nodes); err != nil {
		return fmt.Errorf("invalid partition nodes: %w", err)
	}
	systemIdentifier := x.SystemDescriptionRecord.SystemIdentifier
	systemDescriptionHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := x.Certificate.Verify(trustBase, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	return nil
}
