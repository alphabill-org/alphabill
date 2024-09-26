package genesis

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrRootValidatorsSize = errors.New("registered root nodes do not match consensus total root nodes")
	ErrGenesisRootIssNil  = errors.New("root genesis record is nil")
	ErrNoRootValidators   = errors.New("root nodes not set")
	ErrConsensusIsNil     = errors.New("consensus is nil")
)

type GenesisRootRecord struct {
	_              struct{}         `cbor:",toarray"`
	Version        types.ABVersion  `json:"version,omitempty"`
	RootValidators []*PublicKeyInfo `json:"root_validators,omitempty"`
	Consensus      *ConsensusParams `json:"consensus,omitempty"`
}

// IsValid only validates Consensus structure and that it signed by the listed root nodes
func (x *GenesisRootRecord) IsValid() error {
	if x == nil {
		return ErrGenesisRootIssNil
	}
	if x.Version == 0 {
		return types.ErrInvalidVersion(x)
	}
	if len(x.RootValidators) == 0 {
		return ErrNoRootValidators
	}
	if x.Consensus == nil {
		return ErrConsensusIsNil
	}
	// 1. Check all registered validator nodes are unique and have all fields set correctly
	err := ValidatorInfoUnique(x.RootValidators)
	if err != nil {
		return err
	}
	// 2. Check the consensus structure
	err = x.Consensus.IsValid()
	if err != nil {
		return fmt.Errorf("consensus parameters not valid: %w", err)
	}
	// 3. Verify that all signatures are valid and from known authors
	verifiers, err := NewValidatorTrustBase(x.RootValidators)
	if err != nil {
		return err
	}
	err = x.Consensus.Verify(verifiers)
	if err != nil {
		return err
	}
	return nil
}

// Verify calls IsValid and makes sure that consensus total number of validators matches number of registered root
// validators and number of signatures in consensus structure
func (x *GenesisRootRecord) Verify() error {
	if x == nil {
		return ErrGenesisRootIssNil
	}
	// 1. Check genesis is valid, all structures are filled correctly and consensus is signed by all validators
	//  listed in RootValidators
	err := x.IsValid()
	if err != nil {
		return err
	}
	// 2. Check nof registered validators vs total number of validators in consensus structure
	if x.Consensus.TotalRootValidators != uint32(len(x.RootValidators)) {
		return ErrRootValidatorsSize
	}
	return nil
}

// FindPubKeyById returns matching PublicKeyInfo matching node id or nil if not found
func (x *GenesisRootRecord) FindPubKeyById(id string) *PublicKeyInfo {
	if x == nil {
		return nil
	}
	// linear search for id
	for _, info := range x.RootValidators {
		if info.NodeIdentifier == id {
			return info
		}
	}
	return nil
}

func (x *GenesisRootRecord) GetVersion() types.ABVersion {
	return x.Version
}

func (x *GenesisRootRecord) GetTag() types.ABTag {
	return types.GenesisRootRecordTag
}

func (x *GenesisRootRecord) MarshalCBOR() ([]byte, error) {
	type alias GenesisRootRecord
	return types.Cbor.MarshalTaggedValue(x.GetTag(), (*alias)(x))
}

func (x *GenesisRootRecord) UnmarshalCBOR(data []byte) error {
	type alias GenesisRootRecord
	return types.Cbor.UnmarshalTaggedValue(x.GetTag(), data, (*alias)(x))
}
