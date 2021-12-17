package domain

import (
	"bytes"
	"hash"

	"github.com/holiman/uint256"
)

type (
	// Normaliser type of object can calculate normalised byte format usable by cryptographic operations.
	Normaliser interface {
		Normalise() []byte
	}

	TransactionOrder struct {
		TransactionId         *uint256.Int
		TransactionAttributes Normaliser
		Timeout               uint64
		OwnerProof            Predicate
	}
)

func (o *TransactionOrder) Bytes() []byte {
	var b bytes.Buffer

	if o.TransactionId != nil {
		b32 := o.TransactionId.Bytes32()
		b.Write(b32[:])
	} else {
		b32 := [32]byte{}
		b.Write(b32[:])
	}

	if o.TransactionAttributes != nil {
		b.Write(o.TransactionAttributes.Normalise())
	}

	b.Write(Uint64ToBytes(o.Timeout))

	if o.OwnerProof != nil {
		b.Write(o.OwnerProof)
	}

	return b.Bytes()
}

func (o *TransactionOrder) Hash(hasher hash.Hash) []byte {
	hasher.Write(o.Bytes())
	return hasher.Sum(nil)
}
