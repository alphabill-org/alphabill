package state

import "bytes"

const (
	PaymentTypeTransfer PaymentType = 1
	PaymentTypeSplit    PaymentType = 2
	PaymentTypeJoin     PaymentType = 3
)

// TODO protobuf
type (
	PaymentOrder struct {
		BillID            uint64
		Type              PaymentType
		JoinBillId        uint64
		Amount            uint32
		PayeePredicate    Predicate
		Backlink          []byte
		PredicateArgument Predicate
	}

	PaymentType uint8
)

func (o PaymentOrder) Bytes() []byte {
	var bytes bytes.Buffer
	bytes.Write(Uint64ToBytes(o.BillID))
	bytes.WriteByte(byte(o.Type))
	bytes.Write(Uint64ToBytes(o.JoinBillId))
	bytes.Write(Uint32ToBytes(o.Amount))
	bytes.Write(o.PayeePredicate)
	bytes.Write(o.Backlink)
	bytes.Write(o.PredicateArgument)
	return bytes.Bytes()
}

// Canonical serialization for signature generation, serializes all fields except PredicateArgument
func (o *PaymentOrder) SigBytes() []byte {
	var b bytes.Buffer
	b.Write(Uint64ToBytes(o.BillID))
	b.WriteByte(byte(o.Type))
	b.Write(Uint64ToBytes(o.JoinBillId))
	b.Write(Uint32ToBytes(o.Amount))
	b.Write(o.Backlink)
	b.Write(o.PayeePredicate) // PayeePredicate must be serialized last
	return b.Bytes()
}
