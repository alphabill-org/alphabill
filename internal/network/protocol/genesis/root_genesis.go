package genesis

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var (
	ErrRootGenesisIsNil    = errors.New("root genesis is nil")
	ErrVerifierIsNil       = errors.New("verifier is nil")
	ErrPartitionsNotFound  = errors.New("partitions not found")
	ErrMissingConsensusSig = errors.New("missing consensus signature")
	ErrMissingPubKeyInfo   = errors.New("missing rood validator info")
	ErrRootClusterIsNil    = errors.New("root cluster is nil")
)

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
		return err
	}
	// Check the root cluster is valid
	x.RootCluster.IsValid()
	// verify that the signing key was added
	pubKeyInfo := x.RootCluster.FindPubKeyById(rootId)
	if pubKeyInfo == nil {
		return ErrMissingPubKeyInfo
	}
	// Compare keys
	if !bytes.Equal(pubKeyBytes, pubKeyInfo.SigningPublicKey) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, pubKeyInfo.SigningPublicKey)
	}
	// calculate consensus hash
	alg := gocrypto.Hash(x.RootCluster.Consensus.HashAlgorithm)
	if len(x.Partitions) == 0 {
		return ErrPartitionsNotFound
	}
	for _, p := range x.Partitions {
		if err = p.IsValid(verifier, alg); err != nil {
			return err
		}
	}
	return nil
}

func (x *RootGenesis) IsValidFinal() error {
	if x == nil {
		return ErrRootGenesisIsNil
	}
	// Verify that the consensus struct has been signed by all root nodes
	err := x.RootCluster.IsValidFinal()
	if err != nil {
		return errors.Wrap(err, "Invalid root genesis")
	}
	// todo: verify partition records UC must be signed by all root nodes
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
