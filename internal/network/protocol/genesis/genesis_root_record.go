package genesis

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrNotSignedByAllRootValidators = errors.New("Consensus parameters are not signed by all validators")
var ErrRootValidatorsSize = errors.New("Registered root validators do not match consensus total root nodes")
var ErrGenesisRootIssNil = errors.New("Root genesis record is nil")
var ErrNoRootValidators = errors.New("No root validators set")

func validatorsUnique(validators []*PublicKeyInfo) error {
	if len(validators) == 0 {
		return ErrNoRootValidators
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

// IsValid only validates Consensus structure and the signature of one
func (x *GenesisRootCluster) IsValid() error {
	if x == nil {
		return ErrGenesisRootIssNil
	}
	// 1. Check all registered validator nodes are unique and have all fields set correctly
	err := validatorsUnique(x.RootValidators)
	if err != nil {
		return err
	}
	// 2. Check the consensus structure
	err = x.Consensus.IsValid()
	if err != nil {
		return err
	}
	// 3. Verify that all validators have signed the consensus structure
	// calculate hash of consensus structure
	hash := x.Consensus.Hash(gocrypto.Hash(x.Consensus.HashAlgorithm))
	for _, validator := range x.RootValidators {
		// find signature
		sig, f := x.Consensus.Signatures[validator.NodeIdentifier]
		if !f {
			return errors.Errorf("Consensus struct is not signed by validator %v", validator.NodeIdentifier)
		}
		ver, err := crypto.NewVerifierSecp256k1(validator.SigningPublicKey)
		if err != nil {
			return err
		}
		err = ver.VerifyHash(sig, hash)
		if err != nil {
			return errors.Errorf("Consensus struct signature verification failed for validator %v", validator.NodeIdentifier)
		}
	}
	return nil
}

func (x *GenesisRootCluster) IsValidFinal() error {
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
	// 3. Check number of signatures on consensus struct, it is required to be signed by every validator
	if x.Consensus.TotalRootValidators != uint32(len(x.Consensus.Signatures)) {
		return ErrNotSignedByAllRootValidators
	}
	return nil
}

func (x *GenesisRootCluster) FindPubKeyById(id string) *PublicKeyInfo {
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
