package statedb

import (
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/ethereum/go-ethereum/common"
)

// CreateAccountAndAddCredit - creates EMV account from add fee credit
func CreateAccountAndAddCredit(addr common.Address, owner types.PredicateBytes, balance *big.Int, timeout uint64, transactionRecordHash []byte) state.Action {
	id := addr.Bytes()
	stateObj := &StateObject{
		Address: addr,
		Account: &Account{
			Nonce:    0,
			Balance:  balance,
			CodeHash: emptyCodeHash},
		Storage: map[common.Hash]common.Hash{},
		AlphaBill: &AlphaBillLink{
			TxHash:  transactionRecordHash,
			Timeout: timeout,
		},
	}
	return state.AddUnit(id, owner, stateObj)
}

// UpdateEthAccountAddCredit - increments the balance and updates free credit link
func UpdateEthAccountAddCredit(id types.UnitID, value *big.Int, timeout uint64, transactionRecordHash []byte) state.Action {
	updateDataFunc := func(data state.UnitData) (state.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		newBalance := new(big.Int).Add(stateObj.Account.Balance, value)
		stateObj.Account.Balance = newBalance
		stateObj.AlphaBill = &AlphaBillLink{
			TxHash:  transactionRecordHash,
			Timeout: max(stateObj.AlphaBill.GetTimeout(), timeout),
		}
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// UpdateEthAccountCloseCredit - decrements the balance and updates free credit link
func UpdateEthAccountCloseCredit(id types.UnitID, value *big.Int, txHash []byte) state.Action {
	updateDataFunc := func(data state.UnitData) (state.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		newBalance := new(big.Int).Sub(stateObj.Account.Balance, value)
		stateObj.Account.Balance = newBalance
		stateObj.AlphaBill = &AlphaBillLink{
			TxHash:  txHash,
			Timeout: stateObj.AlphaBill.GetTimeout(),
		}
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}

// SetBalance - set balance to value
func SetBalance(id types.UnitID, value *big.Int) state.Action {
	updateDataFunc := func(data state.UnitData) (state.UnitData, error) {
		stateObj, ok := data.(*StateObject)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain ethereum account", id)
		}
		stateObj.Account.Balance = value
		return stateObj, nil
	}
	return state.UpdateUnitData(id, updateDataFunc)
}
