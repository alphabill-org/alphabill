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
	PartitionDescription *types.PartitionDescriptionRecord `json:"partitionDescriptionRecord"`
	Validators           []*PartitionNode                  `json:"validators"`
}

func (x *PartitionRecord) GetPartitionDescriptionRecord() *types.PartitionDescriptionRecord {
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
	id := x.GetPartitionID()
	var irBytes []byte
	var err error
	for _, node := range x.Validators {
		if err := node.IsValid(); err != nil {
			return fmt.Errorf("validators list error, %w", err)
		}
		if id != node.BlockCertificationRequest.Partition {
			return fmt.Errorf("invalid partition id: expected %s, got %s", id, node.BlockCertificationRequest.Partition)
		}
		// Input record of different validator nodes must match
		// remember first
		if irBytes == nil {
			irBytes, err = node.BlockCertificationRequest.InputRecord.Bytes()
			if err != nil {
				return fmt.Errorf("partition id %s node %v input record error: %w", id, node.BlockCertificationRequest.NodeID, err)
			}
			continue
		}
		// more than one node, compare input record to fist node record
		nextIrBytes, err := node.BlockCertificationRequest.InputRecord.Bytes()
		if err != nil {
			return fmt.Errorf("partition id %s node %v input record error: %w", id, node.BlockCertificationRequest.NodeID, err)
		}
		if !bytes.Equal(irBytes, nextIrBytes) {
			return fmt.Errorf("partition id %s node %v input record is different", id, node.BlockCertificationRequest.NodeID)
		}
	}
	if err = nodesUnique(x.Validators); err != nil {
		return fmt.Errorf("validator list error, %w", err)
	}
	return nil
}

func (x *PartitionRecord) GetPartitionID() types.PartitionID {
	return x.PartitionDescription.PartitionID
}

func (x *PartitionRecord) GetPartitionNode(nodeID string) *PartitionNode {
	for _, v := range x.Validators {
		if v.NodeID == nodeID {
			return v
		}
	}
	return nil
}
