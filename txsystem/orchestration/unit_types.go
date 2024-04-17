package orchestration

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 1
)

var (
	VarUnitType = []byte{0x40}
)

// NewVarID return new Validator Assignment Record ID
func NewVarID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, VarUnitType)
}

// NewVarData return new Validator Assignment Record Unit Data
func NewVarData(unitID types.UnitID) (types.UnitData, error) {
	if unitID.HasType(VarUnitType) {
		return &VarData{}, nil
	}
	return nil, fmt.Errorf("unknown unit type in UnitID %s", unitID)
}
