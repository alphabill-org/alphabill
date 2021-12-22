package domain

import (
	"bytes"
	"crypto"
	"github.com/holiman/uint256"
)

type (
	// TxAttributes type of object can calculate normalised byte format usable by cryptographic operations.
	TxAttributes interface {
		Bytes() []byte
		Value() uint64
		OwnerCondition() []byte
	}

	Transaction struct {
		UnitId *uint256.Int
		//Type                  TransactionType
		TransactionAttributes TxAttributes
		Timeout               uint64
		OwnerProof            Predicate
	}
)

func (o *Transaction) Bytes() []byte {
	var b bytes.Buffer

	if o.UnitId != nil {
		b32 := o.UnitId.Bytes32()
		b.Write(b32[:])
	} else {
		b32 := [32]byte{}
		b.Write(b32[:])
	}

	if o.TransactionAttributes != nil {
		b.Write(o.TransactionAttributes.Bytes())
	}

	b.Write(Uint64ToBytes(o.Timeout))

	if o.OwnerProof != nil {
		b.Write(o.OwnerProof)
	}

	return b.Bytes()
}

func (o *Transaction) Hash() []byte {
	hasher := crypto.SHA256.New()
	hasher.Write(o.Bytes())
	return hasher.Sum(nil)
}
