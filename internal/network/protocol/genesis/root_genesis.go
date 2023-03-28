package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"
)

const (
	RootRound uint64 = 1
)

var (
	ErrRootGenesisIsNil       = errors.New("root genesis is nil")
	ErrRootGenesisRecordIsNil = errors.New("root genesis record is nil")
	ErrVerifierIsNil          = errors.New("verifier is nil")
	ErrPartitionsNotFound     = errors.New("root genesis has no partitions records")
)

type SystemDescriptionRecordGetter interface {
	GetSystemDescriptionRecord() *SystemDescriptionRecord
}

func CheckPartitionSystemIdentifiersUnique[T SystemDescriptionRecordGetter](records []T) error {
	var ids = make(map[string][]byte)
	for _, rec := range records {
		id := rec.GetSystemDescriptionRecord().GetSystemIdentifier()
		if _, f := ids[string(id)]; f {
			return fmt.Errorf("duplicated system identifier: %X", id)
		}
		ids[string(id)] = id
	}
	return nil
}

// IsValid verifies that the genesis file is signed by the generator and that the public key is included
func (x *RootGenesis) IsValid() error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	if x.Root == nil {
		return ErrRootGenesisRecordIsNil
	}
	// check the root genesis record is valid
	err := x.Root.IsValid()
	if err != nil {
		return fmt.Errorf("root genesis record verification failed: %w", err)
	}
	// Verify that UC Seal has been correctly signed
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	// Check that all partition id's are unique
	if err = CheckPartitionSystemIdentifiersUnique(x.Partitions); err != nil {
		return fmt.Errorf("root genesis duplicate partition record error: %w", err)
	}
	trustBase, err := NewValidatorTrustBase(x.Root.RootValidators)
	for _, p := range x.Partitions {
		if err = p.IsValid(trustBase, alg); err != nil {
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
	err := x.Root.Verify()
	if err != nil {
		return fmt.Errorf("root genesis record error: %w", err)
	}
	// Check that the number of signatures on partition UC Seal matches the number of root validators
	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	// Check that all partition id's are unique
	if err = CheckPartitionSystemIdentifiersUnique(x.Partitions); err != nil {
		return fmt.Errorf("root genesis duplicate partition error: %w", err)
	}
	// Check all signatures on Partition UC Seals
	verifiers, err := NewValidatorTrustBase(x.Root.RootValidators)
	if err != nil {
		return fmt.Errorf("root genesis verify failed, unable to create trust base: %w", err)
	}
	// Use hash algorithm from consensus structure
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	for i, p := range x.Partitions {
		if err = p.IsValid(verifiers, alg); err != nil {
			return fmt.Errorf("root genesis partition record %v error: %w", i, err)
		}
		// make sure all root validators have signed the UC Seal
		if len(p.Certificate.UnicitySeal.Signatures) != len(x.Root.RootValidators) {
			return fmt.Errorf("partition %X UC Seal is not signed by all root validators",
				p.SystemDescriptionRecord.SystemIdentifier)
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

func (x *RootGenesis) GetPartitionRecords() []*PartitionRecord {
	records := make([]*PartitionRecord, len(x.Partitions))
	for i, partition := range x.Partitions {
		records[i] = &PartitionRecord{
			SystemDescriptionRecord: partition.SystemDescriptionRecord,
			Validators:              partition.Nodes,
		}
	}
	return records
}
