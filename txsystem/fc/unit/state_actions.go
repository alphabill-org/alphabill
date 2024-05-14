package unit

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

// AddCredit adds a new credit record
func AddCredit(id types.UnitID, owner types.PredicateBytes, data *fc.FeeCreditRecord) state.Action {
	return state.AddUnit(id, owner, data)
}

// DelCredit deletes the credit record
func DelCredit(id types.UnitID) state.Action {
	return state.DeleteUnit(id)
}

// IncrCredit increments the balance of given FeeCreditRecord
func IncrCredit(id types.UnitID, value uint64, timeout uint64) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		fcr, ok := data.(*fc.FeeCreditRecord)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain fee credit record", id)
		}
		return &fc.FeeCreditRecord{
			Balance: fcr.Balance + value,
			Counter: fcr.Counter + 1,
			Timeout: max(fcr.Timeout, timeout),
			Locked:  0,
		}, nil
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
		// note that only balance field changes in this operation
		return &fc.FeeCreditRecord{
			Balance: fcr.Balance - value,
			Counter: fcr.Counter,
			Timeout: fcr.Timeout,
			Locked:  fcr.Locked,
		}, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}
