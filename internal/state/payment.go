package state

import (
	"bytes"
	"hash"
)

const (
	PaymentTypeTransfer PaymentType = 1
	PaymentTypeSplit    PaymentType = 2
)

type (
	PaymentOrder struct {
		Type              PaymentType
		BillID            uint64
		Amount            uint32
		Backlink          []byte
		PayeePredicate    Predicate
		PredicateArgument Predicate
	}

	PaymentType uint8
)

func (o *PaymentOrder) Bytes() []byte {
	var bytes bytes.Buffer
	bytes.WriteByte(byte(o.Type))
	bytes.Write(Uint64ToBytes(o.BillID))
	bytes.Write(Uint32ToBytes(o.Amount))
	bytes.Write(o.Backlink)
	bytes.Write(o.PayeePredicate)
	return bytes.Bytes()
}

func (o *PaymentOrder) Hash(hasher hash.Hash) []byte {
	hasher.Write(o.Bytes())
	return hasher.Sum(nil)
}

// Canonical serialization for signature generation, serializes all fields except PredicateArgument
func (o *PaymentOrder) SigBytes() []byte {
	var b bytes.Buffer
	b.WriteByte(byte(o.Type))
	b.Write(Uint64ToBytes(o.BillID))
	b.Write(Uint32ToBytes(o.Amount))
	b.Write(o.Backlink)
	b.Write(o.PayeePredicate) // PayeePredicate must be serialized last
	return b.Bytes()
}
