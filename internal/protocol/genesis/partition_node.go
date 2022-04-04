package genesis

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var (
	ErrPartitionNodeIsNil  = errors.New("partition node is nil")
	ErrNodeIdentifierIsNil = errors.New("node identifier is empty")
	ErrPublicKeyIsNil      = errors.New("public key is nil")
)

func (x *PartitionNode) IsValid() error {
	if x == nil {
		return ErrPartitionNodeIsNil
	}
	if x.NodeIdentifier == "" {
		return ErrNodeIdentifierIsNil
	}
	if x.PublicKey == nil {
		return ErrPublicKeyIsNil
	}
	pubKey, err := crypto.NewVerifierSecp256k1(x.PublicKey)
	if err != nil {
		return err
	}
	if err := x.P1Request.IsValid(pubKey); err != nil {
		return err
	}
	return nil
}

func nodesUnique(x []*PartitionNode) error {
	var ids = make(map[string]string)
	var keys = make(map[string][]byte)
	for _, node := range x {
		if err := node.IsValid(); err != nil {
			return err
		}
		id := node.NodeIdentifier
		if _, f := ids[id]; f {
			return errors.Errorf("duplicated node id: %v", id)
		}
		ids[id] = id

		pubKey := string(node.PublicKey)
		if _, f := keys[pubKey]; f {
			return errors.Errorf("duplicated node  public key: %X", node.PublicKey)
		}
		keys[pubKey] = node.PublicKey
	}
	return nil
}
