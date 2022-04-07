package txsystem

import (
	"crypto"
	"github.com/holiman/uint256"
)

type (
	GenericTransaction interface {
		SystemID() []byte
		UnitID() *uint256.Int
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
		SigBytes() []byte
	}
)
