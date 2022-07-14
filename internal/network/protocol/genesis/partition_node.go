package genesis

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var (
	ErrPartitionNodeIsNil           = errors.New("partition node is nil")
	ErrNodeIdentifierIsEmpty        = errors.New("node identifier is empty")
	ErrSigningPublicKeyIsInvalid    = errors.New("signing public key is invalid")
	ErrEncryptionPublicKeyIsInvalid = errors.New("encryption public key is invalid")
)

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
		return err
	}
	if len(x.EncryptionPublicKey) == 0 {
		return ErrEncryptionPublicKeyIsInvalid
	}
	_, err = crypto.NewVerifierSecp256k1(x.EncryptionPublicKey)
	if err != nil {
		return err
	}
	if err := x.BlockCertificationRequest.IsValid(signingPubKey); err != nil {
		return err
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
			return errors.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		signingPubKey := string(node.SigningPublicKey)
		if _, f := signingKeys[signingPubKey]; f {
			return errors.Errorf("duplicated node signing public key: %X", node.SigningPublicKey)
		}
		signingKeys[signingPubKey] = node.SigningPublicKey

		encPubKey := string(node.EncryptionPublicKey)
		if _, f := encryptionKeys[encPubKey]; f {
			return errors.Errorf("duplicated node encryption public key: %X", node.EncryptionPublicKey)
		}
		encryptionKeys[encPubKey] = node.EncryptionPublicKey
	}
	return nil
}
