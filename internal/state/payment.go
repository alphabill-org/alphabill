package state

import "bytes"

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
		PredicateArgument Predicate
		PayeePredicate    Predicate
	}

	PaymentType uint8
)

func (o *PaymentOrder) Bytes() []byte {
	var bytes bytes.Buffer
	bytes.WriteByte(byte(o.Type))
	bytes.Write(Uint64ToBytes(o.BillID))
	bytes.Write(Uint32ToBytes(o.Amount))
	bytes.Write(o.Backlink)
	bytes.Write(o.PredicateArgument)
	return bytes.Bytes()
}
