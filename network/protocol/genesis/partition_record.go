package genesis

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	errPartitionRecordIsNil = errors.New("partition record is nil")
	errValidatorsMissing    = errors.New("validators are missing")
)

type PartitionRecord struct {
	_                    struct{}                          `cbor:",toarray"`
	PartitionDescription *types.PartitionDescriptionRecord `json:"partition_description_record,omitempty"`
	Validators           []*PartitionNode                  `json:"validators,omitempty"`
}

func (x *PartitionRecord) GetSystemDescriptionRecord() *types.PartitionDescriptionRecord {
	if x == nil {
		return nil
	}
	return x.PartitionDescription
}

func (x *PartitionRecord) IsValid() error {
	if x == nil {
		return errPartitionRecordIsNil
	}
	if err := x.PartitionDescription.IsValid(); err != nil {
		return fmt.Errorf("invalid system description record, %w", err)
	}
	if len(x.Validators) == 0 {
		return errValidatorsMissing
	}
	id := x.GetSystemIdentifier()
	var irBytes []byte
	for _, node := range x.Validators {
		if err := node.IsValid(); err != nil {
			return fmt.Errorf("validators list error, %w", err)
		}
		if id != node.BlockCertificationRequest.Partition {
			return fmt.Errorf("invalid system id: expected %s, got %s", id, node.BlockCertificationRequest.Partition)
		}
		// Input record of different validator nodes must match
		// remember first
		if irBytes == nil {
			irBytes = node.BlockCertificationRequest.InputRecord.Bytes()
			continue
		}
		// more than one node, compare input record to fist node record
		if !bytes.Equal(irBytes, node.BlockCertificationRequest.InputRecord.Bytes()) {
			return fmt.Errorf("system id %s node %v input record is different", id, node.BlockCertificationRequest.NodeIdentifier)
		}
	}
	if err := nodesUnique(x.Validators); err != nil {
		return fmt.Errorf("validator list error, %w", err)
	}
	return nil
}

func (x *PartitionRecord) GetSystemIdentifier() types.SystemID {
	return x.PartitionDescription.SystemIdentifier
}

func (x *PartitionRecord) GetPartitionNode(id string) *PartitionNode {
	for _, v := range x.Validators {
		if v.NodeIdentifier == id {
			return v
		}
	}
	return nil
}
