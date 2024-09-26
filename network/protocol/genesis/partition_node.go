package genesis

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

var (
	ErrPartitionNodeIsNil           = errors.New("partition node is nil")
	ErrNodeIdentifierIsEmpty        = errors.New("node identifier is empty")
	ErrSigningPublicKeyIsInvalid    = errors.New("signing public key is invalid")
	ErrEncryptionPublicKeyIsInvalid = errors.New("encryption public key is invalid")
)

type PartitionNode struct {
	_                         struct{}                                 `cbor:",toarray"`
	Version                   types.ABVersion                          `json:"version,omitempty"`
	NodeIdentifier            string                                   `json:"node_identifier,omitempty"`
	SigningPublicKey          []byte                                   `json:"signing_public_key,omitempty"`
	EncryptionPublicKey       []byte                                   `json:"encryption_public_key,omitempty"`
	BlockCertificationRequest *certification.BlockCertificationRequest `json:"block_certification_request,omitempty"`
	Params                    []byte                                   `json:"params,omitempty"`
	PartitionDescription      types.PartitionDescriptionRecord         `json:"partition_description"`
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
	if x.NodeIdentifier == "" {
		return ErrNodeIdentifierIsEmpty
	}
	if len(x.SigningPublicKey) == 0 {
		return ErrSigningPublicKeyIsInvalid
	}
	signingPubKey, err := crypto.NewVerifierSecp256k1(x.SigningPublicKey)
	if err != nil {
		return fmt.Errorf("invalid signing public key, %w", err)
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrEncryptionPublicKeyIsInvalid
	}
	if _, err = crypto.NewVerifierSecp256k1(x.EncryptionPublicKey); err != nil {
		return fmt.Errorf("invalid encryption public key, %w", err)
	}
	if err = x.BlockCertificationRequest.IsValid(signingPubKey); err != nil {
		return fmt.Errorf("block certification request validation failed, %w", err)
	}
	return nil
}

func nodesUnique(x []*PartitionNode) error {
	var ids = make(map[string]string)
	var signingKeys = make(map[string][]byte)
	var encryptionKeys = make(map[string][]byte)
	for _, node := range x {
		if err := node.IsValid(); err != nil {
			return err
		}
		id := node.NodeIdentifier
		if _, f := ids[id]; f {
			return fmt.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		signingPubKey := string(node.SigningPublicKey)
		if _, f := signingKeys[signingPubKey]; f {
			return fmt.Errorf("duplicated node signing public key: %X", node.SigningPublicKey)
		}
		signingKeys[signingPubKey] = node.SigningPublicKey

		encPubKey := string(node.EncryptionPublicKey)
		if _, f := encryptionKeys[encPubKey]; f {
			return fmt.Errorf("duplicated node encryption public key: %X", node.EncryptionPublicKey)
		}
		encryptionKeys[encPubKey] = node.EncryptionPublicKey
	}
	return nil
}
