package genesis

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
)

var (
	errPartitionRecordIsNil = errors.New("partition record is nil")
	errValidatorsMissing    = errors.New("validators are missing")
)

func (x *PartitionRecord) IsValid() error {
	if x == nil {
		return errPartitionRecordIsNil
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return fmt.Errorf("invalid system description record, %w", err)
	}
	if len(x.Validators) == 0 {
		return errValidatorsMissing
	}
	id := x.GetSystemIdentifier()
	var ir *certificates.InputRecord = nil
	for _, node := range x.Validators {
		if err := node.IsValid(); err != nil {
			return fmt.Errorf("validators list error, %w", err)
		}
		if !bytes.Equal(id, node.BlockCertificationRequest.SystemIdentifier) {
			return fmt.Errorf("invalid system id: expected %X, got %X", id, node.BlockCertificationRequest.SystemIdentifier)
		}
		// Input record of different validator nodes must match
		// remember first
		if ir == nil {
			ir = node.BlockCertificationRequest.InputRecord
			continue
		}
		// more than one node, compare input record to fist node record
		if !bytes.Equal(ir.Bytes(), node.BlockCertificationRequest.InputRecord.Bytes()) {
			return fmt.Errorf("system id %X node %v input record is different", id, node.BlockCertificationRequest.NodeIdentifier)
		}
	}
	if err := nodesUnique(x.Validators); err != nil {
		return fmt.Errorf("validator list error, %w", err)
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
