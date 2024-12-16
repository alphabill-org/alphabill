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
	ErrValidatorPublicInfoIsEmpty = errors.New("public key info is empty")
	ErrPubKeyNodeIDIsEmpty        = errors.New("public key info node identifier is empty")
	ErrPubKeyInfoSignKeyIsInvalid = errors.New("public key info signing key is invalid")
	ErrPubKeyInfoAuthKeyIsInvalid = errors.New("public key info authentication key is invalid")
)

type PublicKeyInfo struct {
	_       struct{}  `cbor:",toarray"`
	NodeID  string    `json:"nodeId"`
	AuthKey hex.Bytes `json:"authKey"`
	SignKey hex.Bytes `json:"signKey"`
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
		ver, err := abcrypto.NewVerifierSecp256k1(info.SignKey)
		if err != nil {
			return nil, err
		}
		nodeIdToKey[info.NodeID] = ver
	}
	return nodeIdToKey, nil
}

// TODO: This is the same functionality as for PartitionNode, by defining a common interface these could be combined into one method, thus removing duplicate code.

// ValidatorInfoUnique checks for duplicates in the slice, makes sure that there are no validators that share the same
// id or public key. There is one exception, currently a validator can use the same key for authentication and signing.
func ValidatorInfoUnique(validators []*PublicKeyInfo) error {
	if len(validators) == 0 {
		return ErrValidatorPublicInfoIsEmpty
	}
	var ids = make(map[string]string)
	var signKeys = make(map[string]hex.Bytes)
	var authKeys = make(map[string]hex.Bytes)
	for _, nodeInfo := range validators {
		if err := nodeInfo.IsValid(); err != nil {
			return err
		}
		id := nodeInfo.NodeID
		if _, f := ids[id]; f {
			return fmt.Errorf("duplicate node id: %v", id)
		}
		ids[id] = id

		signKey := string(nodeInfo.SignKey)
		if _, f := signKeys[signKey]; f {
			return fmt.Errorf("duplicate node signing key: %X", nodeInfo.SignKey)
		}
		signKeys[signKey] = nodeInfo.SignKey

		authKey := string(nodeInfo.AuthKey)
		if _, f := authKeys[authKey]; f {
			return fmt.Errorf("duplicate node authentication key: %X", nodeInfo.AuthKey)
		}
		authKeys[authKey] = nodeInfo.AuthKey
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
	if len(x.SignKey) == 0 {
		return ErrPubKeyInfoSignKeyIsInvalid
	}
	if _, err := abcrypto.NewVerifierSecp256k1(x.SignKey); err != nil {
		return fmt.Errorf("invalid signing key: %w", err)
	}
	if len(x.AuthKey) == 0 {
		return ErrPubKeyInfoAuthKeyIsInvalid
	}
	if _, err := abcrypto.NewVerifierSecp256k1(x.AuthKey); err != nil {
		return fmt.Errorf("invalid authentication key: %w", err)
	}
	return nil
}

// GetNodeID - returns node identifier as peer.ID from authentication key
// The NodeID (string) could also be used with Decode(),
// but there are a lot of unit tests that init the field as "test" or to some other invalid id
func (x *PublicKeyInfo) GetNodeID() (peer.ID, error) {
	pKey, err := p2pcrypto.UnmarshalSecp256k1PublicKey(x.AuthKey)
	if err != nil {
		return "", fmt.Errorf("authentication key marshal error: %w", err)
	}
	return peer.IDFromPublicKey(pKey)
}
