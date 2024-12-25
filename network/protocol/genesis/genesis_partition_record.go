package genesis

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
	ErrTrustBaseIsNil              = errors.New("trust base is nil")
)

type GenesisPartitionRecord struct {
	_                    struct{}                          `cbor:",toarray"`
	Version              types.ABVersion                   `json:"version"`
	Validators           []*PartitionNode                  `json:"validators"`
	Certificate          *types.UnicityCertificate         `json:"certificate"`
	PartitionDescription *types.PartitionDescriptionRecord `json:"partitionDescriptionRecord"`
}

func (x *GenesisPartitionRecord) GetPartitionDescriptionRecord() *types.PartitionDescriptionRecord {
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
	if len(x.Validators) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.PartitionDescription.IsValid(); err != nil {
		return fmt.Errorf("invalid partition description record: %w", err)
	}
	if err := nodesUnique(x.Validators); err != nil {
		return fmt.Errorf("invalid partition nodes: %w", err)
	}
	if err := x.hasNodesConsensus(); err != nil {
		return fmt.Errorf("invalid partition nodes: %w", err)
	}

	if trustBase != nil {
		partitionID := x.PartitionDescription.PartitionID
		partitionDescriptionHash, err := x.PartitionDescription.Hash(hashAlgorithm)
		if err != nil {
			return fmt.Errorf("partition description hash error: %w", err)
		}
		if err := x.Certificate.Verify(trustBase, hashAlgorithm, partitionID, partitionDescriptionHash); err != nil {
			return fmt.Errorf("invalid unicity certificate: %w", err)
		}
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

func nodesUnique(x []*PartitionNode) error {
	var ids = make(map[string]string)
	var signKeys = make(map[string]hex.Bytes)
	var authKeys = make(map[string]hex.Bytes)
	for _, node := range x {
		if err := node.IsValid(); err != nil {
			return err
		}
		id := node.NodeID
		if _, f := ids[id]; f {
			return fmt.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		sigKey := string(node.SignKey)
		if _, f := signKeys[sigKey]; f {
			return fmt.Errorf("duplicated node signing key: %X", node.SignKey)
		}
		signKeys[sigKey] = node.SignKey

		authKey := string(node.AuthKey)
		if _, f := authKeys[authKey]; f {
			return fmt.Errorf("duplicated node authentication key: %X", node.AuthKey)
		}
		authKeys[authKey] = node.AuthKey
	}
	return nil
}

func (x *GenesisPartitionRecord) hasNodesConsensus() error {
	partitionID := x.PartitionDescription.PartitionID
	var prevIRBytes []byte
	for _, node := range x.Validators {
		nodeID := node.BlockCertificationRequest.NodeID
		nodePartitionID := node.BlockCertificationRequest.PartitionID
		if partitionID != nodePartitionID {
			return fmt.Errorf("partition id %s node %v invalid blockCertificationRequest partition id %s", partitionID, nodeID, nodePartitionID)
		}

		irBytes, err := node.BlockCertificationRequest.InputRecord.Bytes()
		if err != nil {
			return fmt.Errorf("partition id %s node %v input record error: %w", partitionID, nodeID, err)
		}
		if prevIRBytes != nil && !bytes.Equal(irBytes, prevIRBytes) {
			return fmt.Errorf("partition id %s node %v input record is different", partitionID, nodeID)
		}
		prevIRBytes = irBytes
	}
	return nil
}
