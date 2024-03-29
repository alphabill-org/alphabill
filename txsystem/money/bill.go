package money

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
)

type BillData struct {
	_        struct{}    `cbor:",toarray"`
	V        uint64      `json:"value,string"`      // The monetary value of this bill
	T        uint64      `json:"lastUpdate,string"` // The round number of the last transaction with the bill
	Backlink types.Bytes `json:"backlink"`          // Backlink (256-bit hash)
	Locked   uint64      `json:"locked,string"`     // locked status of the bill, non-zero value means locked
}

func (b *BillData) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(b)
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
