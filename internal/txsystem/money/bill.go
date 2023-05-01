package money

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type BillData struct {
	V        uint64 // The monetary value of this bill
	T        uint64 // The round number of the last transaction with the bill
	Backlink []byte // Backlink (256-bit hash)
}

type InitialBill struct {
	ID    *uint256.Int
	Value uint64
	Owner rma.Predicate
}

func (b *BillData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.V))
	hasher.Write(util.Uint64ToBytes(b.T))
	hasher.Write(b.Backlink)
}

func (b *BillData) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(b.V)
}
