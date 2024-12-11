package unit

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
)

// AddCredit adds a new credit record
func AddCredit(id types.UnitID, data *fc.FeeCreditRecord) state.Action {
	return state.AddUnit(id, data)
}

// DelCredit deletes the credit record
func DelCredit(id types.UnitID) state.Action {
	return state.DeleteUnit(id)
}

// IncrCredit increments the balance of given FeeCreditRecord
func IncrCredit(id types.UnitID, value uint64, minLifetime uint64) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		fcr, ok := data.(*fc.FeeCreditRecord)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain fee credit record", id)
		}
		balance, ok := util.SafeAdd(fcr.Balance, value)
		if !ok {
			return nil, errors.New("failed to increment balance: overflow")
		}
		fcr.Balance = balance
		fcr.Locked = 0 // FCR is automatically unlocked on AddFC
		fcr.Counter += 1
		fcr.MinLifetime = max(fcr.MinLifetime, minLifetime)
		return fcr, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// DecrCredit decrements the balance of given FeeCreditRecord
func DecrCredit(id types.UnitID, value uint64) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		fcr, ok := data.(*fc.FeeCreditRecord)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain fee credit record", id)
		}
		balance, ok := util.SafeSub(fcr.Balance, value)
		if !ok {
			return nil, errors.New("failed to decrement balance: underflow")
		}
		// note that only balance field changes in this operation
		fcr.Balance = balance
		return fcr, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}
