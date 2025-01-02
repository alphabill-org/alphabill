package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

var (
	ErrPartitionGenesisIsNil            = errors.New("partition genesis is nil")
	ErrPartitionValidatorsMissing       = errors.New("partition validators are missing")
	ErrPartitionUnicityCertificateIsNil = errors.New("partition unicity certificate is nil")
)

type PartitionGenesis struct {
	_                    struct{}                          `cbor:",toarray"`
	PartitionDescription *types.PartitionDescriptionRecord `json:"partitionDescriptionRecord"`
	Certificate          *types.UnicityCertificate         `json:"certificate"`
	PartitionValidators  []*types.NodeInfo                 `json:"partitionValidators"`
	Params               hex.Bytes                         `json:"params,omitempty"`
}

func (x *PartitionGenesis) IsValid(trustBase types.RootTrustBase, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrPartitionGenesisIsNil
	}
	if trustBase == nil {
		return ErrTrustBaseIsNil
	}
	if len(x.PartitionValidators) < 1 {
		return ErrPartitionValidatorsMissing
	}
	if err := validateNodes(x.PartitionValidators); err != nil {
		return fmt.Errorf("invalid partition validators, %w", err)
	}
	if x.PartitionDescription == nil {
		return types.ErrSystemDescriptionIsNil
	}
	if err := x.PartitionDescription.IsValid(); err != nil {
		return fmt.Errorf("invalid partition description record, %w", err)
	}
	if x.Certificate == nil {
		return ErrPartitionUnicityCertificateIsNil
	}
	pdrHash, err := x.PartitionDescription.Hash(hashAlgorithm)
	if err != nil {
		return fmt.Errorf("partition description hash error, %w", err)
	}
	// validate all signatures against known root keys
	if err := x.Certificate.Verify(trustBase, hashAlgorithm, x.PartitionDescription.PartitionID, pdrHash); err != nil {
		return fmt.Errorf("invalid unicity certificate, %w", err)
	}
	return nil
}
