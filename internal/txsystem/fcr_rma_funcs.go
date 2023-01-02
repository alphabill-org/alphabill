package txsystem

import (
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

// AddCredit adds a new credit record
func AddCredit(id *uint256.Int, owner rma.Predicate, data *FeeCreditRecord, stateHash []byte) rma.Action {
	return rma.AddItem(id, owner, data, stateHash)
}

// DelCredit deletes the credit record
func DelCredit(id *uint256.Int) rma.Action {
	return rma.DeleteItem(id)
}

// IncrCredit increments the balance of given FeeCreditRecord
func IncrCredit(id *uint256.Int, value int64, timeout uint64, stateHash []byte) rma.Action {
	updateDataFunc := func(data rma.UnitData) (newData rma.UnitData) {
		fcr, ok := data.(*FeeCreditRecord)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		return &FeeCreditRecord{
			balance: fcr.balance + value,
			hash:    stateHash,
			timeout: util.Max(fcr.timeout, timeout),
		}
	}
	return rma.UpdateData(id, updateDataFunc, stateHash)
}

// DecrCredit decrements the balance of given FeeCreditRecord
func DecrCredit(id *uint256.Int, value int64, stateHash []byte) rma.Action {
	updateDataFunc := func(data rma.UnitData) (newData rma.UnitData) {
		fcr, ok := data.(*FeeCreditRecord)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		// note that hash and timeout remain unchanged in this operation
		return &FeeCreditRecord{
			balance: fcr.balance - value,
			hash:    fcr.hash,
			timeout: fcr.timeout,
		}
	}
	return rma.UpdateData(id, updateDataFunc, stateHash)
}
