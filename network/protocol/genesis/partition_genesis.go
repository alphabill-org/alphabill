package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	ErrPartitionGenesisIsNil            = errors.New("partition genesis is nil")
	ErrKeysAreMissing                   = errors.New("partition keys are missing")
	ErrMissingRootValidators            = errors.New("missing root nodes")
	ErrPartitionUnicityCertificateIsNil = errors.New("partition unicity certificate is nil")
)

type PartitionGenesis struct {
	_                    struct{}                          `cbor:",toarray"`
	PartitionDescription *types.PartitionDescriptionRecord `json:"partitionDescriptionRecord,omitempty"`
	Certificate          *types.UnicityCertificate         `json:"certificate,omitempty"`
	RootValidators       []*PublicKeyInfo                  `json:"rootValidators,omitempty"`
	Keys                 []*PublicKeyInfo                  `json:"keys,omitempty"`
	Params               []byte                            `json:"params,omitempty"`
}

func (x *PartitionGenesis) FindRootPubKeyInfoById(id string) *PublicKeyInfo {
	// linear search for id
	for _, info := range x.RootValidators {
		if info.NodeIdentifier == id {
			return info
		}
	}
	return nil
}

func (x *PartitionGenesis) IsValid(trustBase types.RootTrustBase, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrPartitionGenesisIsNil
	}
	if trustBase == nil {
		return ErrTrustBaseIsNil
	}
	if len(x.Keys) < 1 {
		return ErrKeysAreMissing
	}
	if len(x.RootValidators) < 1 {
		return ErrMissingRootValidators
	}
	// check that root validators are valid and
	// make sure it is a list of unique node ids and keys
	if err := ValidatorInfoUnique(x.RootValidators); err != nil {
		return fmt.Errorf("root node list validation failed, %w", err)
	}
	// check partition validator public info is valid, and
	// it is a list of unique node ids and keys
	if err := ValidatorInfoUnique(x.Keys); err != nil {
		return fmt.Errorf("partition keys validation failed, %w", err)
	}

	if x.PartitionDescription == nil {
		return types.ErrSystemDescriptionIsNil
	}
	if err := x.PartitionDescription.IsValid(); err != nil {
		return fmt.Errorf("invalid system decsrition record, %w", err)
	}
	if x.Certificate == nil {
		return ErrPartitionUnicityCertificateIsNil
	}
	sdrHash := x.PartitionDescription.Hash(hashAlgorithm)
	// validate all signatures against known root keys
	if err := x.Certificate.Verify(trustBase, hashAlgorithm, x.PartitionDescription.PartitionIdentifier, sdrHash); err != nil {
		return fmt.Errorf("invalid unicity certificate, %w", err)
	}
	// UC Seal must be signed by all validators
	if len(x.RootValidators) != len(x.Certificate.UnicitySeal.Signatures) {
		return fmt.Errorf("unicity Certificate is not signed by all root nodes")
	}
	return nil
}

// GenerateRootTrustBase generates trust base from partition genesis.
func (x *PartitionGenesis) GenerateRootTrustBase() (types.RootTrustBase, error) {
	if x == nil {
		return nil, ErrPartitionGenesisIsNil
	}
	nodes, err := newTrustBaseNodes(x.RootValidators)
	if err != nil {
		return nil, err
	}
	trustBase, err := types.NewTrustBaseGenesis(nodes, x.Certificate.UnicitySeal.Hash)
	if err != nil {
		return nil, err
	}
	return trustBase, nil
}

func newTrustBaseNodes(publicKeyInfo []*PublicKeyInfo) ([]*types.NodeInfo, error) {
	var nodeInfo []*types.NodeInfo
	for _, info := range publicKeyInfo {
		verifier, err := crypto.NewVerifierSecp256k1(info.SigningPublicKey)
		if err != nil {
			return nil, err
		}
		nodeInfo = append(nodeInfo, types.NewNodeInfo(info.NodeIdentifier, 1, verifier))
	}
	return nodeInfo, nil
}
