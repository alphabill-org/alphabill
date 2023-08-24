package unit

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

// AddCredit adds a new credit record
func AddCredit(id types.UnitID, owner state.Predicate, data *FeeCreditRecord) state.Action {
	return state.AddUnit(id, owner, data)
}

// DelCredit deletes the credit record
func DelCredit(id types.UnitID) state.Action {
	return state.DeleteUnit(id)
}

// IncrCredit increments the balance of given FeeCreditRecord
func IncrCredit(id types.UnitID, value uint64, timeout uint64, transactionRecordHash []byte) state.Action {
	updateDataFunc := func(data state.UnitData) (state.UnitData, error) {
		fcr, ok := data.(*FeeCreditRecord)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain fee credit record", id)
		}
		return &FeeCreditRecord{
			Balance: fcr.Balance + value,
			Hash:    bytes.Clone(transactionRecordHash), //
			Timeout: util.Max(fcr.Timeout, timeout),
		}, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// DecrCredit decrements the balance of given FeeCreditRecord
func DecrCredit(id types.UnitID, value uint64) state.Action {
	updateDataFunc := func(data state.UnitData) (state.UnitData, error) {
		fcr, ok := data.(*FeeCreditRecord)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain fee credit record", id)
		}
		// note that hash and timeout remain unchanged in this operation
		return &FeeCreditRecord{
			Balance: fcr.Balance - value,
			Hash:    fcr.Hash,
			Timeout: fcr.Timeout,
		}, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}
