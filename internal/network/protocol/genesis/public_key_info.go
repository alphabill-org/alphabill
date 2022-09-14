package genesis

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var (
	ErrValidatorPublicInfoIsEmpty = errors.New("public key info is empty")
)

func NewRootTrustBase(rootPublicInfo []*PublicKeyInfo) (map[string]crypto.Verifier, error) {
	if len(rootPublicInfo) == 0 {
		return nil, ErrValidatorPublicInfoIsEmpty
	}
	// Collect all root signing keys
	rootIdToKey := make(map[string]crypto.Verifier)
	for _, info := range rootPublicInfo {
		ver, err := crypto.NewVerifierSecp256k1(info.SigningPublicKey)
		if err != nil {
			return nil, errors.New("invalid signing public key")
		}
		rootIdToKey[info.NodeIdentifier] = ver
	}
	return rootIdToKey, nil
}

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
			return errors.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		signingPubKey := string(nodeInfo.SigningPublicKey)
		if _, f := signingKeys[signingPubKey]; f {
			return errors.Errorf("duplicated node signing public key: %X", nodeInfo.SigningPublicKey)
		}
		signingKeys[signingPubKey] = nodeInfo.SigningPublicKey

		encPubKey := string(nodeInfo.EncryptionPublicKey)
		if _, f := encryptionKeys[encPubKey]; f {
			return errors.Errorf("duplicated node encryption public key: %X", nodeInfo.EncryptionPublicKey)
		}
		encryptionKeys[encPubKey] = nodeInfo.EncryptionPublicKey
	}
	return nil
}

func (x *PublicKeyInfo) IsValid() error {
	if x == nil {
		return ErrValidatorPublicInfoIsEmpty
	}
	if x.NodeIdentifier == "" {
		return ErrNodeIdentifierIsEmpty
	}
	if len(x.SigningPublicKey) == 0 {
		return ErrSigningPublicKeyIsInvalid
	}
	_, err := crypto.NewVerifierSecp256k1(x.SigningPublicKey)
	if err != nil {
		return err
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrEncryptionPublicKeyIsInvalid
	}
	_, err = crypto.NewVerifierSecp256k1(x.EncryptionPublicKey)
	if err != nil {
		return err
	}
	return nil
}
