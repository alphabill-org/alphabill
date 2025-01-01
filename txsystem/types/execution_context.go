package types

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type (
	StateInfo interface {
		GetUnit(id types.UnitID, committed bool) (state.VersionedUnit, error)
		CurrentRound() uint64
	}

	FeeCalculation interface {
		BuyGas(tema uint64) uint64
		CalculateCost(spentGas uint64) uint64
	}

	// TxExecutionContext - implementation of ExecutionContext interface for generic tx handler
	TxExecutionContext struct {
		txo          *types.TransactionOrder
		txs          StateInfo
		fee          FeeCalculation
		trustStore   types.RootTrustBase
		initialGas   uint64
		remainingGas uint64
		customData   []byte
	}
)

func (ec *TxExecutionContext) GetUnit(id types.UnitID, committed bool) (state.VersionedUnit, error) {
	return ec.txs.GetUnit(id, committed)
}

func (ec *TxExecutionContext) CurrentRound() uint64 { return ec.txs.CurrentRound() }

func (ec *TxExecutionContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return ec.trustStore, nil
}

func (ec *TxExecutionContext) GasAvailable() uint64 {
	return ec.remainingGas
}

func (ec *TxExecutionContext) SpendGas(gas uint64) error {
	if gas > ec.remainingGas {
		ec.remainingGas = 0
		return types.ErrOutOfGas
	}
	ec.remainingGas -= gas
	return nil
}

func (ec *TxExecutionContext) CalculateCost() uint64 {
	gasUsed := ec.initialGas - ec.remainingGas
	cost := ec.fee.CalculateCost(gasUsed)
	return cost
}

func (ec *TxExecutionContext) TransactionOrder() (*types.TransactionOrder, error) {
	if ec.txo == nil {
		return nil, types.ErrTransactionOrderIsNil
	}
	return ec.txo, nil
}

func (ec *TxExecutionContext) GetData() []byte {
	return ec.customData
}

func (ec *TxExecutionContext) SetData(data []byte) {
	ec.customData = data
}

func NewExecutionContext(txo *types.TransactionOrder, txSys StateInfo, f FeeCalculation, tb types.RootTrustBase, maxCost uint64) *TxExecutionContext {
	gasUnits := f.BuyGas(maxCost)
	return &TxExecutionContext{
		txo:          txo,
		txs:          txSys,
		fee:          f,
		trustStore:   tb,
		initialGas:   gasUnits,
		remainingGas: gasUnits,
	}
}
