package evm

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
)

// NB! EVM does not have unit type currently, UnitID is ethereum address

func NewUnitData(unitID types.UnitID) (types.UnitData, error) {
	return &statedb.StateObject{}, nil
}
