package genesis

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	RootRound uint64 = 1
	Timestamp uint64 = 1668208271000 // 11.11.2022 @ 11:11:11
)

var (
	ErrRootGenesisIsNil       = errors.New("root genesis is nil")
	ErrRootGenesisRecordIsNil = errors.New("root genesis record is nil")
	ErrPartitionsNotFound     = errors.New("root genesis has no partitions records")
)

type RootGenesis struct {
	_          struct{}                  `cbor:",toarray"`
	Version    types.ABVersion           `json:"version"`
	Root       *GenesisRootRecord        `json:"root"`
	Partitions []*GenesisPartitionRecord `json:"partitions"`
}

type PartitionDescriptionRecordGetter interface {
	GetPartitionDescriptionRecord() *types.PartitionDescriptionRecord
}

func CheckPartitionPartitionIDsUnique[T PartitionDescriptionRecordGetter](records []T) error {
	ids := make(map[types.PartitionID]struct{}, len(records))
	for _, rec := range records {
		record := rec.GetPartitionDescriptionRecord()
		if _, f := ids[record.PartitionID]; f {
			return fmt.Errorf("duplicate partition identifier: %s", record.PartitionID)
		}
		ids[record.PartitionID] = struct{}{}
	}
	return nil
}

// IsValid verifies that the genesis file is signed by the generator and that the public key is included
func (x *RootGenesis) IsValid() error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	if x.Version == 0 {
		return types.ErrInvalidVersion(x)
	}
	if x.Root == nil {
		return ErrRootGenesisRecordIsNil
	}
	if err := x.Root.IsValid(); err != nil {
		return fmt.Errorf("root genesis record verification failed: %w", err)
	}

	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	// Check that all partition id's are unique
	if err := CheckPartitionPartitionIDsUnique(x.Partitions); err != nil {
		return fmt.Errorf("root genesis duplicate partition record error: %w", err)
	}
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	trustBase, err := x.GenerateTrustBase()
	if err != nil {
		return fmt.Errorf("creating validator trustbase: %w", err)
	}
	for _, p := range x.Partitions {
		if err := p.Verify(trustBase, alg); err != nil {
			return err
		}
	}
	return nil
}

// Verify basically same as IsValid, but verifies that the consensus structure and UC Seals are signed by all root
// validators
func (x *RootGenesis) Verify() error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	if x.Root == nil {
		return ErrRootGenesisRecordIsNil
	}
	// Verify that the root genesis record is valid and signed by all validators
	if err := x.Root.Verify(); err != nil {
		return fmt.Errorf("invalid root partition record: %w", err)
	}
	// Check that the number of signatures on partition UC Seal matches the number of root validators
	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	// Check that all partition id's are unique
	if err := CheckPartitionPartitionIDsUnique(x.Partitions); err != nil {
		return fmt.Errorf("invalid partitions: %w", err)
	}
	// Check all signatures on Partition UC Seals
	trustBase, err := x.GenerateTrustBase()
	if err != nil {
		return fmt.Errorf("failed to create trust base: %w", err)
	}
	// Use hash algorithm from consensus structure
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	for i, p := range x.Partitions {
		if err = p.Verify(trustBase, alg); err != nil {
			return fmt.Errorf("invalid partition record at index %v: %w", i, err)
		}
		// make sure all root validators have signed the UC Seal
		if len(p.Certificate.UnicitySeal.Signatures) != len(x.Root.RootValidators) {
			return fmt.Errorf("partition %X UC Seal is not signed by all root nodes",
				p.PartitionDescription.PartitionID)
		}
	}
	return nil
}

func (x *RootGenesis) GetRoundNumber() uint64 {
	return x.Partitions[0].Certificate.UnicitySeal.RootChainRoundNumber
}

func (x *RootGenesis) GetRoundHash() []byte {
	return x.Partitions[0].Certificate.UnicitySeal.Hash
}

/*
NodeIDs returns IDs of all root validator nodes.
*/
func (x *RootGenesis) NodeIDs() ([]peer.ID, error) {
	IDs := make([]peer.ID, len(x.Root.RootValidators))
	for n, v := range x.Root.RootValidators {
		id, err := peer.Decode(v.NodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to convert node ID %q: %w", v.NodeID, err)
		}
		IDs[n] = id
	}
	return IDs, nil
}

// GenerateTrustBase generates root trust base. The final trust
// base must be generated from the combined root genesis file.
func (x *RootGenesis) GenerateTrustBase(opts ...types.Option) (*types.RootTrustBaseV1, error) {
	var unicityTreeRootHash []byte
	// sanity check that unicity tree root hashes are equal for all partitions
	for _, p := range x.Partitions {
		if len(unicityTreeRootHash) == 0 {
			unicityTreeRootHash = p.Certificate.UnicitySeal.Hash
		} else if !bytes.Equal(unicityTreeRootHash, p.Certificate.UnicitySeal.Hash) {
			return nil, errors.New("unicity certificate seal hashes are not equal")
		}
	}
	trustBase, err := types.NewTrustBaseGenesis(x.Root.RootValidators, unicityTreeRootHash, opts...)
	if err != nil {
		return nil, err
	}
	return trustBase, nil
}

func (x *RootGenesis) GetVersion() types.ABVersion {
	return x.Version
}

func (x *RootGenesis) MarshalCBOR() ([]byte, error) {
	type alias RootGenesis
	return types.Cbor.MarshalTaggedValue(types.RootGenesisTag, (*alias)(x))
}

func (x *RootGenesis) UnmarshalCBOR(data []byte) error {
	type alias RootGenesis
	return types.Cbor.UnmarshalTaggedValue(types.RootGenesisTag, data, (*alias)(x))
}

func (x *RootGenesis) GetPartitionGenesis(partitionID types.PartitionID) (*GenesisPartitionRecord, error) {
	for _, pg := range x.Partitions {
		if pg.PartitionDescription.PartitionID == partitionID {
			return pg, nil
		}
	}
	return nil, fmt.Errorf("partition %q not found in root genesis", partitionID)
}
