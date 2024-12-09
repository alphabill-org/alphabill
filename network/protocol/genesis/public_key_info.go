package genesis

import (
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

var (
	ErrValidatorPublicInfoIsEmpty    = errors.New("public key info is empty")
	ErrPubKeyNodeIDIsEmpty           = errors.New("public key info node identifier is empty")
	ErrPubKeyInfoSigningKeyIsInvalid = errors.New("public key info singing key is invalid")
	ErrPubKeyInfoEncryptionIsInvalid = errors.New("public key info encryption key is invalid")
)

type PublicKeyInfo struct {
	_                   struct{}  `cbor:",toarray"`
	NodeID              string    `json:"nodeId"`
	SigningPublicKey    hex.Bytes `json:"signingPublicKey"`
	EncryptionPublicKey hex.Bytes `json:"encryptionPublicKey"`
}

// NewValidatorTrustBase creates a verifier to node id map from public key info using the signing public key.
func NewValidatorTrustBase(publicKeyInfo []*PublicKeyInfo) (map[string]abcrypto.Verifier, error) {
	// If is nil or empty - return the same error
	if len(publicKeyInfo) == 0 {
		return nil, ErrValidatorPublicInfoIsEmpty
	}
	// Create a map of all validator node identifier to verifier (public signing keys)
	nodeIdToKey := make(map[string]abcrypto.Verifier)
	for _, info := range publicKeyInfo {
		ver, err := abcrypto.NewVerifierSecp256k1(info.SigningPublicKey)
		if err != nil {
			return nil, err
		}
		nodeIdToKey[info.NodeID] = ver
	}
	return nodeIdToKey, nil
}

// TODO: This is the same functionality as for PartitionNode, by defining a common interface these could be combined into one method, thus removing duplicate code.

// ValidatorInfoUnique checks for duplicates in the slice, makes sure that there are no validators that share the same
// id or public key. There is one exception, currently a validator can use the same key for encryption and signing.
func ValidatorInfoUnique(validators []*PublicKeyInfo) error {
	if len(validators) == 0 {
		return ErrValidatorPublicInfoIsEmpty
	}
	var ids = make(map[string]string)
	var signingKeys = make(map[string]hex.Bytes)
	var encryptionKeys = make(map[string]hex.Bytes)
	for _, nodeInfo := range validators {
		if err := nodeInfo.IsValid(); err != nil {
			return err
		}
		id := nodeInfo.NodeID
		if _, f := ids[id]; f {
			return fmt.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		signingPubKey := string(nodeInfo.SigningPublicKey)
		if _, f := signingKeys[signingPubKey]; f {
			return fmt.Errorf("duplicated node signing public key: %X", nodeInfo.SigningPublicKey)
		}
		signingKeys[signingPubKey] = nodeInfo.SigningPublicKey

		encPubKey := string(nodeInfo.EncryptionPublicKey)
		if _, f := encryptionKeys[encPubKey]; f {
			return fmt.Errorf("duplicated node encryption public key: %X", nodeInfo.EncryptionPublicKey)
		}
		encryptionKeys[encPubKey] = nodeInfo.EncryptionPublicKey
	}
	return nil
}

// IsValid validates that all fields are correctly set and public keys are correct.
func (x *PublicKeyInfo) IsValid() error {
	if x == nil {
		return ErrValidatorPublicInfoIsEmpty
	}
	if x.NodeID == "" {
		return ErrPubKeyNodeIDIsEmpty
	}
	if len(x.SigningPublicKey) == 0 {
		return ErrPubKeyInfoSigningKeyIsInvalid
	}
	if _, err := abcrypto.NewVerifierSecp256k1(x.SigningPublicKey); err != nil {
		return fmt.Errorf("invalid signing key: %w", err)
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrPubKeyInfoEncryptionIsInvalid
	}
	if _, err := abcrypto.NewVerifierSecp256k1(x.EncryptionPublicKey); err != nil {
		return fmt.Errorf("invalid encryption key: %w", err)
	}
	return nil
}

// GetNodeID - returns node identifier as peer.ID from encryption public key
// The NodeID (string) could also be used with Decode(),
// but there are a lot of unit tests that init the field as "test" or to some other invalid id
func (x *PublicKeyInfo) GetNodeID() (peer.ID, error) {
	pKey, err := p2pcrypto.UnmarshalSecp256k1PublicKey(x.EncryptionPublicKey)
	if err != nil {
		return "", fmt.Errorf("encryption key marshal error: %w", err)
	}
	return peer.IDFromPublicKey(pKey)
}
