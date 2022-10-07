package genesis

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

const (
	ErrRootGenesisIsNil       = "root genesis is nil"
	ErrRootGenesisRecordIsNil = "root genesis record is nil"
	ErrVerifierIsNil          = "verifier is nil"
	ErrPartitionsNotFound     = "partitions not found"
)

type SystemDescriptionRecordGetter interface {
	GetSystemDescriptionRecord() *SystemDescriptionRecord
}

func CheckPartitionSystemIdentifiersUnique[T SystemDescriptionRecordGetter](records []T) error {
	var ids = make(map[string][]byte)
	for _, rec := range records {
		id := rec.GetSystemDescriptionRecord().GetSystemIdentifier()
		if _, f := ids[string(id)]; f {
			return errors.Errorf("duplicated system identifier: %X", id)
		}
		ids[string(id)] = id
	}
	return nil
}

// IsValid verifies that the genesis file is signed by the generator and that the public key is included
func (x *RootGenesis) IsValid(rootId string, verifier crypto.Verifier) error {
	if x == nil {
		return errors.New(ErrRootGenesisIsNil)
	}
	if verifier == nil {
		return errors.New(ErrVerifierIsNil)
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return err
	}
	if x.Root == nil {
		return errors.New(ErrRootGenesisRecordIsNil)
	}
	// check the root genesis record is valid
	err = x.Root.IsValid()
	if err != nil {
		return errors.Wrap(err, "root genesis record verification failed")
	}
	// verify that the signing public key is present in root validator info
	pubKeyInfo := x.Root.FindPubKeyById(rootId)
	if pubKeyInfo == nil {
		return errors.Errorf("missing public key info for node id %v", rootId)
	}
	// Compare keys
	if !bytes.Equal(pubKeyBytes, pubKeyInfo.SigningPublicKey) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, pubKeyInfo.SigningPublicKey)
	}
	// Verify that UC Seal has been correctly signed
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	if len(x.Partitions) == 0 {
		return errors.New(ErrPartitionsNotFound)
	}
	// Check that all partition id's are unique
	if err := CheckPartitionSystemIdentifiersUnique(x.Partitions); err != nil {
		return err
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
		return errors.New(ErrRootGenesisIsNil)
	}
	if x.Root == nil {
		return errors.New(ErrRootGenesisRecordIsNil)
	}
	// Verify that the root genesis record is valid and signed by all validators
	err := x.Root.Verify()
	if err != nil {
		return errors.Wrap(err, "invalid root genesis record")
	}
	// Check that the number of signatures on partition UC Seal matches the number of root validators
	if len(x.Partitions) == 0 {
		return errors.New(ErrPartitionsNotFound)
	}
	// Check that all partition id's are unique
	if err := CheckPartitionSystemIdentifiersUnique(x.Partitions); err != nil {
		return errors.Wrap(err, "invalid root genesis record")
	}
	// Check all signatures on Partition UC Seals
	verifiers, err := NewValidatorTrustBase(x.Root.RootValidators)
	if err != nil {
		return errors.Wrap(err, "invalid root genesis validators")
	}
	// Use hash algorithm from consensus structure
	alg := gocrypto.Hash(x.Root.Consensus.HashAlgorithm)
	for _, p := range x.Partitions {
		if err = p.IsValid(verifiers, alg); err != nil {
			return err
		}
		// make sure all root validators have signed the UC Seal
		if len(p.Certificate.UnicitySeal.Signatures) != len(x.Root.RootValidators) {
			return errors.Errorf("Partition %X UC Seal is not signed by all root validators",
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
