package genesis

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

var (
	ErrPartitionNodeIsNil = errors.New("partition node is nil")
	ErrNodeIDIsEmpty      = errors.New("node identifier is empty")
	ErrSignKeyIsInvalid   = errors.New("signing key is invalid")
	ErrAuthKeyIsInvalid   = errors.New("authentication key is invalid")
)

type PartitionNode struct {
	_                          struct{}                                 `cbor:",toarray"`
	Version                    types.ABVersion                          `json:"version"`
	NodeID                     string                                   `json:"nodeId"`
	AuthKey                    hex.Bytes                                `json:"authKey"`
	SignKey                    hex.Bytes                                `json:"signKey"`
	BlockCertificationRequest  *certification.BlockCertificationRequest `json:"blockCertificationRequest"`
	PartitionDescriptionRecord types.PartitionDescriptionRecord         `json:"partitionDescriptionRecord"`
	Params                     hex.Bytes                                `json:"params,omitempty"`
}

type MoneyPartitionParams struct {
	_          struct{} `cbor:",toarray"`
	Partitions []*types.PartitionDescriptionRecord
}

type EvmPartitionParams struct {
	_             struct{} `cbor:",toarray"`
	BlockGasLimit uint64
	GasUnitPrice  uint64
}

type OrchestrationPartitionParams struct {
	_              struct{} `cbor:",toarray"`
	OwnerPredicate types.PredicateBytes
}

type TokensPartitionParams struct {
	_                   struct{} `cbor:",toarray"`
	AdminOwnerPredicate []byte
	FeelessMode         bool
}

func (x *PartitionNode) IsValid() error {
	if x == nil {
		return ErrPartitionNodeIsNil
	}
	if x.Version == 0 {
		return types.ErrInvalidVersion(x)
	}
	if x.NodeID == "" {
		return ErrNodeIDIsEmpty
	}
	if len(x.SignKey) == 0 {
		return ErrSignKeyIsInvalid
	}
	signKey, err := crypto.NewVerifierSecp256k1(x.SignKey)
	if err != nil {
		return fmt.Errorf("invalid signing public key, %w", err)
	}
	if len(x.AuthKey) == 0 {
		return ErrAuthKeyIsInvalid
	}
	if _, err = crypto.NewVerifierSecp256k1(x.AuthKey); err != nil {
		return fmt.Errorf("invalid authentication public key, %w", err)
	}
	if err = x.BlockCertificationRequest.IsValid(signKey); err != nil {
		return fmt.Errorf("block certification request validation failed, %w", err)
	}
	return nil
}

func nodesUnique(x []*PartitionNode) error {
	var ids = make(map[string]string)
	var signKeys = make(map[string]hex.Bytes)
	var authKeys = make(map[string]hex.Bytes)
	for _, node := range x {
		if err := node.IsValid(); err != nil {
			return err
		}
		id := node.NodeID
		if _, f := ids[id]; f {
			return fmt.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		sigKey := string(node.SignKey)
		if _, f := signKeys[sigKey]; f {
			return fmt.Errorf("duplicated node signing key: %X", node.SignKey)
		}
		signKeys[sigKey] = node.SignKey

		authKey := string(node.AuthKey)
		if _, f := authKeys[authKey]; f {
			return fmt.Errorf("duplicated node authentication key: %X", node.AuthKey)
		}
		authKeys[authKey] = node.AuthKey
	}
	return nil
}

func (x *PartitionNode) GetVersion() types.ABVersion {
	return x.Version
}

func (x *PartitionNode) MarshalCBOR() ([]byte, error) {
	type alias PartitionNode
	return types.Cbor.MarshalTaggedValue(types.PartitionNodeTag, (*alias)(x))
}

func (x *PartitionNode) UnmarshalCBOR(data []byte) error {
	type alias PartitionNode
	return types.Cbor.UnmarshalTaggedValue(types.PartitionNodeTag, data, (*alias)(x))
}
