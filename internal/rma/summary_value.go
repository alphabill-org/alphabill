package rma

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

type (

	// SummaryValue is different from UnitData. It is derived from UnitData with UnitData.Value function.
	SummaryValue interface {
		// AddToHasher adds the value of summary value to the hasher.
		AddToHasher(hasher hash.Hash)
		// Concatenate calculates new SummaryValue by concatenating this, left and right.
		Concatenate(left, right SummaryValue) SummaryValue
		// Bytes returns bytes of the SummaryValue
		Bytes() []byte
	}

	Uint64SummaryValue uint64
)

func (t Uint64SummaryValue) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(t)))
}

func (t Uint64SummaryValue) Concatenate(left, right SummaryValue) SummaryValue {
	var s, l, r uint64
	s = uint64(t)
	lSum, ok := left.(Uint64SummaryValue)
	if ok {
		l = uint64(lSum)
	}
	rSum, ok := right.(Uint64SummaryValue)
	if ok {
		r = uint64(rSum)
	}
	return Uint64SummaryValue(s + l + r)
}

func (t Uint64SummaryValue) Value() uint64 {
	return uint64(t)
}

func (t Uint64SummaryValue) Bytes() []byte {
	return util.Uint64ToBytes(uint64(t))
}
