package genesis

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrValidatorPublicInfoIsEmpty    = errors.New("public key info is empty")
	ErrPubKeyNodeIdentifierIsEmpty   = errors.New("public key info node identifier is empty")
	ErrPubKeyInfoSigningKeyIsInvalid = errors.New("public key info singing key is invalid")
	ErrPubKeyInfoEncryptionIsInvalid = errors.New("public key info encryption key is invalid")
)

// NewValidatorTrustBase creates a verifier to node id map from public key info using the signing public key.
func NewValidatorTrustBase(publicKeyInfo []*PublicKeyInfo) (map[string]crypto.Verifier, error) {
	// If is nil or empty - return the same error
	if len(publicKeyInfo) == 0 {
		return nil, ErrValidatorPublicInfoIsEmpty
	}
	// Create a map of all validator node identifier to verifier (public signing keys)
	nodeIdToKey := make(map[string]crypto.Verifier)
	for _, info := range publicKeyInfo {
		ver, err := crypto.NewVerifierSecp256k1(info.SigningPublicKey)
		if err != nil {
			return nil, err
		}
		nodeIdToKey[info.NodeIdentifier] = ver
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
	var signingKeys = make(map[string][]byte)
	var encryptionKeys = make(map[string][]byte)
	for _, nodeInfo := range validators {
		if err := nodeInfo.IsValid(); err != nil {
			return err
		}
		id := nodeInfo.NodeIdentifier
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
	if x.NodeIdentifier == "" {
		return ErrPubKeyNodeIdentifierIsEmpty
	}
	if len(x.SigningPublicKey) == 0 {
		return ErrPubKeyInfoSigningKeyIsInvalid
	}
	if _, err := crypto.NewVerifierSecp256k1(x.SigningPublicKey); err != nil {
		return fmt.Errorf("invalid signing key, %w", err)
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrPubKeyInfoEncryptionIsInvalid
	}
	if _, err := crypto.NewVerifierSecp256k1(x.EncryptionPublicKey); err != nil {
		return fmt.Errorf("invalid encryption key, %w", err)
	}
	return nil
}
