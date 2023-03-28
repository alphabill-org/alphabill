package genesis

import (
	"bytes"
	"github.com/alphabill-org/alphabill/internal/certificates"

	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrPartitionRecordIsNil = errors.New("partition record is nil")
var ErrValidatorsMissing = errors.New("validators are missing")

func (x *PartitionRecord) IsValid() error {
	if x == nil {
		return ErrPartitionRecordIsNil
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	if len(x.Validators) == 0 {
		return ErrValidatorsMissing
	}
	id := x.GetSystemIdentifier()
	var ir *certificates.InputRecord = nil
	for _, node := range x.Validators {
		if err := node.IsValid(); err != nil {
			return err
		}
		if !bytes.Equal(id, node.BlockCertificationRequest.SystemIdentifier) {
			return errors.Errorf("invalid system id: expected %X, got %X", id, node.BlockCertificationRequest.SystemIdentifier)
		}
		// Input record of different validator nodes must match
		// remember first
		if ir == nil {
			ir = node.BlockCertificationRequest.InputRecord
			continue
		}
		// more than one node, compare input record to fist node record
		if !bytes.Equal(ir.Bytes(), node.BlockCertificationRequest.InputRecord.Bytes()) {
			return errors.Errorf("system id %X node %v input record is different", id, node.BlockCertificationRequest.NodeIdentifier)
		}
	}
	if err := nodesUnique(x.Validators); err != nil {
		return err
	}
	return nil
}

func (x *PartitionRecord) GetSystemIdentifier() []byte {
	return x.SystemDescriptionRecord.SystemIdentifier
}

func (x *PartitionRecord) GetSystemIdentifierString() string {
	return x.SystemDescriptionRecord.GetSystemIdentifierString()
}

func (x *PartitionRecord) GetPartitionNode(id string) *PartitionNode {
	for _, v := range x.Validators {
		if v.NodeIdentifier == id {
			return v
		}
	}
	return nil
}
