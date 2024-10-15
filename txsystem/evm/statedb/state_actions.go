package statedb

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

// CreateAccountAndAddCredit - creates EMV account from add fee credit
func CreateAccountAndAddCredit(addr common.Address, ownerPredicate types.PredicateBytes, balance *uint256.Int, timeout uint64) state.Action {
	id := addr.Bytes()
	stateObj := &StateObject{
		Address: addr,
		Account: &Account{
			Nonce:    0,
			Balance:  balance,
			CodeHash: emptyCodeHash},
		Storage: map[common.Hash]common.Hash{},
		AlphaBill: &AlphaBillLink{
			Counter:        0,
			Timeout:        timeout,
			OwnerPredicate: ownerPredicate,
		},
	}
	return state.AddUnit(id, stateObj)
}

// UpdateEthAccountAddCredit - increments the balance and updates fee credit link
func UpdateEthAccountAddCredit(id types.UnitID, value *uint256.Int, timeout uint64, newOwnerPredicate []byte) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		newBalance := new(uint256.Int).Add(stateObj.Account.Balance, value)
		stateObj.Account.Balance = newBalance
		stateObj.AlphaBill = &AlphaBillLink{
			Counter:        stateObj.AlphaBill.Counter + 1,
			Timeout:        max(stateObj.AlphaBill.GetTimeout(), timeout),
			OwnerPredicate: newOwnerPredicate,
		}
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// UpdateEthAccountCloseCredit - decrements the balance and updates fee credit link
func UpdateEthAccountCloseCredit(id types.UnitID, value *uint256.Int) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		newBalance := new(uint256.Int).Sub(stateObj.Account.Balance, value)
		stateObj.Account.Balance = newBalance
		stateObj.AlphaBill = &AlphaBillLink{
			Counter: stateObj.AlphaBill.Counter + 1,
			Timeout: stateObj.AlphaBill.GetTimeout(),
		}
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// SetBalance - set balance to value
func SetBalance(id types.UnitID, value *uint256.Int) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		stateObj.Account.Balance = value
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}
