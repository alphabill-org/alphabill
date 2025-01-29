package genesis

import (
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrRootValidatorsSize = errors.New("registered root nodes do not match consensus total root nodes")
	ErrGenesisRootIssNil  = errors.New("root genesis record is nil")
	ErrNoRootValidators   = errors.New("root nodes not set")
	ErrConsensusIsNil     = errors.New("consensus is nil")
)

type GenesisRootRecord struct {
	_              struct{}          `cbor:",toarray"`
	Version        types.ABVersion   `json:"version"`
	RootValidators []*types.NodeInfo `json:"rootValidators"`
	Consensus      *ConsensusParams  `json:"consensus"`
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
	err := validateNodes(x.RootValidators)
	if err != nil {
		return err
	}
	// 2. Check the consensus structure
	err = x.Consensus.IsValid()
	if err != nil {
		return fmt.Errorf("consensus parameters not valid: %w", err)
	}
	// 3. Verify that all signatures are valid and from known authors
	verifiers, err := getVerifiers(x.RootValidators)
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

// FindRootValidatorByNodeID returns NodeInfo for the given nodeID or nil if not found
func (x *GenesisRootRecord) FindRootValidatorByNodeID(nodeID string) *types.NodeInfo {
	if x == nil {
		return nil
	}
	// linear search for id
	for _, info := range x.RootValidators {
		if info.NodeID == nodeID {
			return info
		}
	}
	return nil
}

func (x *GenesisRootRecord) GetVersion() types.ABVersion {
	return x.Version
}

func (x *GenesisRootRecord) MarshalCBOR() ([]byte, error) {
	type alias GenesisRootRecord
	return types.Cbor.MarshalTaggedValue(types.GenesisRootRecordTag, (*alias)(x))
}

func (x *GenesisRootRecord) UnmarshalCBOR(data []byte) error {
	type alias GenesisRootRecord
	return types.Cbor.UnmarshalTaggedValue(types.GenesisRootRecordTag, data, (*alias)(x))
}

func getVerifiers(nodes []*types.NodeInfo) (map[string]abcrypto.Verifier, error) {
	res := make(map[string]abcrypto.Verifier, len(nodes))
	for _, node := range nodes {
		sigVerifier, err := node.SigVerifier()
		if err != nil {
			return nil, err
		}
		res[node.NodeID] = sigVerifier
	}
	return res, nil
}

func validateNodes(nodes []*types.NodeInfo) error {
	var nodeIDs = make(map[string]struct{})
	for _, node := range nodes {
		if err := node.IsValid(); err != nil {
			return err
		}
		if _, f := nodeIDs[node.NodeID]; f {
			return fmt.Errorf("duplicate node id: %v", node.NodeID)
		}
		nodeIDs[node.NodeID] = struct{}{}
	}
	return nil
}
