package orchestration

import (
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
)

// VarData Validator Assignment Record Data
type VarData struct {
	_           struct{} `cbor:",toarray"`
	EpochNumber uint64   // epoch number from the validator assignment record
}

func (b *VarData) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(b)
	if err != nil {
		return fmt.Errorf("validator assignment record data encode error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (b *VarData) SummaryValueInput() uint64 {
	return 0 // no summary value checks in orchestration partition
}

func (b *VarData) Copy() state.UnitData {
	return &VarData{
		EpochNumber: b.EpochNumber,
	}
}
