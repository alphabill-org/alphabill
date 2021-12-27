package domain

import (
	"bytes"
	"hash"
)

const (
	PaymentTypeTransfer PaymentType = 0
	PaymentTypeSplit    PaymentType = 1
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

	Predicate []byte
)

func (o *PaymentOrder) Bytes() []byte {
	var b bytes.Buffer
	b.WriteByte(byte(o.Type))
	b.Write(Uint64ToBytes(o.BillID))
	b.Write(Uint32ToBytes(o.Amount))
	b.Write(o.Backlink)
	b.Write(o.PayeePredicate)
	return b.Bytes()
}

func (o *PaymentOrder) Hash(hasher hash.Hash) []byte {
	hasher.Write(o.Bytes())
	return hasher.Sum(nil)
}
