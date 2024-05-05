package genesis

import (
	gocrypto "crypto"
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

func (x *GenesisPartitionRecord) IsValid(trustBase types.RootTrustBase, hashAlgorithm gocrypto.Hash) error {
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
		return fmt.Errorf("system decrition validation failed, %w", err)
	}
	systemIdentifier := x.SystemDescriptionRecord.SystemIdentifier
	systemDescriptionHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := nodesUnique(x.Nodes); err != nil {
		return fmt.Errorf("partition nodes validation failed, %w", err)
	}
	if err := x.Certificate.Verify(trustBase, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("unicity certificate verify error: %w", err)
	}
	return nil
}
