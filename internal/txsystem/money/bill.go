package money

import (
	"bytes"
	"hash"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

type BillData struct {
	V        uint64 // The monetary value of this bill
	T        uint64 // The round number of the last transaction with the bill
	TDust    uint64 // The last swapDC transaction timeout with the bill
	Backlink []byte // Backlink (256-bit hash)
}

func (b *BillData) Write(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.V))
	hasher.Write(util.Uint64ToBytes(b.T))
	hasher.Write(util.Uint64ToBytes(b.TDust))
	hasher.Write(b.Backlink)
}

func (b *BillData) SummaryValueInput() uint64 {
	return b.V
}

func (b *BillData) Copy() state.UnitData {
	return &BillData{
		V:        b.V,
		T:        b.T,
		TDust:    b.TDust,
		Backlink: bytes.Clone(b.Backlink),
	}
}

type InitialBill struct {
	ID    types.UnitID
	Value uint64
	Owner state.Predicate
}
