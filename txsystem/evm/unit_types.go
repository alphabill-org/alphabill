package evm

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

const (
	UnitIDLength   = UnitPartLength + TypePartLength
	UnitPartLength = 32
	TypePartLength = 0
)

// NB! EVM does not have unit type currently, UnitID is ethereum address

func NewFeeCreditRecordID(shardPart []byte, unitPart []byte) types.UnitID {
	return types.NewUnitID(UnitIDLength, shardPart, unitPart, nil)
}

func NewUnitData(unitID types.UnitID) (types.UnitData, error) {
	return &statedb.StateObject{}, nil
}
