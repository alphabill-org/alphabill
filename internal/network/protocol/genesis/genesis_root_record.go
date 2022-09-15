package genesis

import (
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrNotSignedByAllRootValidators = errors.New("Consensus parameters are not signed by all validators")
var ErrRootValidatorsSize = errors.New("Registered root validators do not match consensus total root nodes")
var ErrGenesisRootIssNil = errors.New("Root genesis record is nil")
var ErrNoRootValidators = errors.New("No root validators set")
var ErrConsensusIsNil = errors.New("consensus is nil")

// IsValid only validates Consensus structure and the signature of one
func (x *GenesisRootRecord) IsValid() error {
	if x == nil {
		return ErrGenesisRootIssNil
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
		return err
	}
	// 3. Verify that all validators have signed the consensus structure
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
	err := x.IsValid()
	if err != nil {
		return err
	}
	// 2. Check nof registered validators vs total number of validators in consensus structure
	if x.Consensus.TotalRootValidators != uint32(len(x.RootValidators)) {
		return ErrRootValidatorsSize
	}
	// 3. Check number of signatures on consensus struct, it is required to be signed by every validator
	if x.Consensus.TotalRootValidators != uint32(len(x.Consensus.Signatures)) {
		return ErrNotSignedByAllRootValidators
	}
	return nil
}

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
