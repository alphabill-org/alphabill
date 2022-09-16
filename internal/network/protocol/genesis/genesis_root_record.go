package genesis

import (
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrRootValidatorsSize = errors.New("registered root validators do not match consensus total root nodes")
var ErrGenesisRootIssNil = errors.New("root genesis record is nil")
var ErrNoRootValidators = errors.New("no root validators set")
var ErrConsensusIsNil = errors.New("consensus is nil")

// IsValid only validates Consensus structure and that it signed by the listed root validators
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
