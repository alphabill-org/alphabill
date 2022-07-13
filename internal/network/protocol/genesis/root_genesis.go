package genesis

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var (
	ErrRootGenesisIsNil   = errors.New("root genesis is nil")
	ErrVerifierIsNil      = errors.New("verifier is nil")
	ErrPartitionsNotFound = errors.New("partitions not found")
)

func (x *RootGenesis) IsValid(verifier crypto.Verifier) error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	if verifier == nil {
		return ErrVerifierIsNil
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(pubKeyBytes, x.TrustBase) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, x.TrustBase)
	}

	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	for _, p := range x.Partitions {
		if err = p.IsValid(verifier, gocrypto.Hash(x.HashAlgorithm)); err != nil {
			return err
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
