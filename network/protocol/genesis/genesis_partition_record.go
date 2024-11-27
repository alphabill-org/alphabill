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
	_                    struct{}                          `cbor:",toarray"`
	Version              types.ABVersion                   `json:"version"`
	Nodes                []*PartitionNode                  `json:"nodes"`
	Certificate          *types.UnicityCertificate         `json:"certificate"`
	PartitionDescription *types.PartitionDescriptionRecord `json:"partitionDescriptionRecord"`
}

func (x *GenesisPartitionRecord) GetSystemDescriptionRecord() *types.PartitionDescriptionRecord {
	if x == nil {
		return nil
	}
	return x.PartitionDescription
}

func (x *GenesisPartitionRecord) IsValid(trustBase types.RootTrustBase, hashAlgorithm crypto.Hash) error {
	if x == nil {
		return ErrGenesisPartitionRecordIsNil
	}
	if x.Version == 0 {
		return types.ErrInvalidVersion(x)
	}
	if trustBase == nil {
		return ErrTrustBaseIsNil
	}
	if len(x.Nodes) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.PartitionDescription.IsValid(); err != nil {
		return fmt.Errorf("invalid system description record: %w", err)
	}
	if err := nodesUnique(x.Nodes); err != nil {
		return fmt.Errorf("invalid partition nodes: %w", err)
	}
	partitionIdentifier := x.PartitionDescription.PartitionIdentifier
	systemDescriptionHash, err := x.PartitionDescription.Hash(hashAlgorithm)
	if err != nil {
		return fmt.Errorf("system description hash error: %w", err)
	}
	if err := x.Certificate.Verify(trustBase, hashAlgorithm, partitionIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	return nil
}

func (x *GenesisPartitionRecord) GetVersion() types.ABVersion {
	return x.Version
}

func (x *GenesisPartitionRecord) MarshalCBOR() ([]byte, error) {
	type alias GenesisPartitionRecord
	return types.Cbor.MarshalTaggedValue(types.GenesisPartitionRecordTag, (*alias)(x))
}

func (x *GenesisPartitionRecord) UnmarshalCBOR(data []byte) error {
	type alias GenesisPartitionRecord
	return types.Cbor.UnmarshalTaggedValue(types.GenesisPartitionRecordTag, data, (*alias)(x))
}
