package genesis

import (
	"bytes"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
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
	for _, node := range x.Validators {
		if err := node.IsValid(); err != nil {
			return err
		}
		if !bytes.Equal(id, node.BlockCertificationRequest.SystemIdentifier) {
			return errors.Errorf("invalid system id: expected %X, got %X", id, node.BlockCertificationRequest.SystemIdentifier)
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
