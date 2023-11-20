package money

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/predicates"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
)

type BillData struct {
	_        struct{} `cbor:",toarray"`
	V        uint64   // The monetary value of this bill
	T        uint64   // The round number of the last transaction with the bill
	Backlink []byte   // Backlink (256-bit hash)
	Locked   uint64   // locked status of the bill, non-zero value means locked
}

func (b *BillData) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(b)
	if err != nil {
		return fmt.Errorf("unit data encode error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (b *BillData) SummaryValueInput() uint64 {
	return b.V
}

func (b *BillData) Copy() state.UnitData {
	return &BillData{
		V:        b.V,
		T:        b.T,
		Backlink: bytes.Clone(b.Backlink),
		Locked:   b.Locked,
	}
}

func (b *BillData) IsLocked() bool {
	return b.Locked != 0
}

type InitialBill struct {
	ID    types.UnitID
	Value uint64
	Owner predicates.PredicateBytes
}
