package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	fc "github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 1
)

var (
	BillUnitType            = []byte{0x00}
	FeeCreditRecordUnitType = []byte{0x0f}
)

func NewBillID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, BillUnitType)
}

func NewFeeCreditRecordID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, FeeCreditRecordUnitType)
}

func NewUnitData(unitID types.UnitID) (state.UnitData, error) {
	if unitID.HasType(BillUnitType) {
		return &BillData{}, nil
	}
	if unitID.HasType(FeeCreditRecordUnitType) {
		return &fc.FeeCreditRecord{}, nil
	}

	return nil, fmt.Errorf("unknown unit type in UnitID %s", unitID)
}
