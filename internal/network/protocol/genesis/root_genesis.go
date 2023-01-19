package genesis

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
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
func (x *RootGenesis) IsValid(rootId string, verifier crypto.Verifier) error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	if verifier == nil {
		return ErrVerifierIsNil
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("invalid verifier, unable to extract public key: %w", err)
	}
	if x.Root == nil {
		return ErrRootGenesisRecordIsNil
	}
	// check the root genesis record is valid
	err = x.Root.IsValid()
	if err != nil {
		return fmt.Errorf("root genesis record verification failed: %w", err)
	}
	// verify that the signing public key is present in root validator info
	pubKeyInfo := x.Root.FindPubKeyById(rootId)
	if pubKeyInfo == nil {
		return fmt.Errorf("missing public key info for node id %v", rootId)
	}
	// Compare keys
	if !bytes.Equal(pubKeyBytes, pubKeyInfo.SigningPublicKey) {
		return fmt.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, pubKeyInfo.SigningPublicKey)
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
	verifiers := map[string]crypto.Verifier{rootId: verifier}
	for _, p := range x.Partitions {
		if err = p.IsValid(verifiers, alg); err != nil {
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
		return fmt.Errorf("root genesis validators, trust base error: %w", err)
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
	return x.Partitions[0].Certificate.UnicitySeal.RootRoundInfo.RoundNumber
}

func (x *RootGenesis) GetRoundHash() []byte {
	return x.Partitions[0].Certificate.UnicitySeal.CommitInfo.RootHash
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
