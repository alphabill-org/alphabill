package genesis

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrGenesisPartitionRecordIsNil = errors.New("genesis partition record is nil")
	ErrNodesAreMissing             = errors.New("nodes are missing")
)

func (x *GenesisPartitionRecord) IsValid(verifier crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {
		return ErrGenesisPartitionRecordIsNil
	}
	if verifier == nil {
		return ErrVerifierIsNil
	}
	if len(x.Nodes) == 0 {
		return ErrNodesAreMissing
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	systemIdentifier := x.SystemDescriptionRecord.SystemIdentifier
	systemDescriptionHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := nodesUnique(x.Nodes); err != nil {
		return err
	}
	if err := x.Certificate.IsValid(verifier, hashAlgorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	return nil
}

func (x *GenesisPartitionRecord) GetSystemIdentifierString() p.SystemIdentifier {
	return p.SystemIdentifier(x.SystemDescriptionRecord.SystemIdentifier)
}
