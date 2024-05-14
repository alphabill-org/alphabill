package statedb

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

// CreateAccountAndAddCredit - creates EMV account from add fee credit
func CreateAccountAndAddCredit(addr common.Address, owner types.PredicateBytes, balance *uint256.Int, timeout uint64) state.Action {
	id := addr.Bytes()
	stateObj := &StateObject{
		Address: addr,
		Account: &Account{
			Nonce:    0,
			Balance:  balance,
			CodeHash: emptyCodeHash},
		Storage: map[common.Hash]common.Hash{},
		AlphaBill: &AlphaBillLink{
			Counter: 0,
			Timeout: timeout,
		},
	}
	return state.AddUnit(id, owner, stateObj)
}

// UpdateEthAccountAddCredit - increments the balance and updates fee credit link
func UpdateEthAccountAddCredit(id types.UnitID, value *uint256.Int, timeout uint64) state.Action {
	updateDataFunc := func(data types.UnitData) (types.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		newBalance := new(uint256.Int).Add(stateObj.Account.Balance, value)
		stateObj.Account.Balance = newBalance
		stateObj.AlphaBill = &AlphaBillLink{
			Counter: stateObj.AlphaBill.Counter + 1,
			Timeout: max(stateObj.AlphaBill.GetTimeout(), timeout),
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
