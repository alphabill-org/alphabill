package evm

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 1
)

var (
	FeeCreditRecordUnitType = []byte{0x0f}
)

func NewFeeCreditRecordID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, FeeCreditRecordUnitType)
}
