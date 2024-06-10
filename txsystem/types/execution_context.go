package types

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

const (
	GeneralTxCostGasUnits = 400
	GasUnitsPerTema       = 1000
)

var ErrOutOfGas = errors.New("out of gas")

type (
	StateInfo interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		CurrentRound() uint64
	}

	// TxExecutionContext - implementation of ExecutionContext interface for generic tx handler
	TxExecutionContext struct {
		txs          StateInfo
		trustStore   types.RootTrustBase
		initialGas   uint64
		remainingGas uint64
	}
)

func (ec *TxExecutionContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return ec.txs.GetUnit(id, committed)
}

func (ec *TxExecutionContext) CurrentRound() uint64 { return ec.txs.CurrentRound() }

func (ec *TxExecutionContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return ec.trustStore, nil
}

// until AB-1012 gets resolved we need this hack to get correct payload bytes.
func (ec *TxExecutionContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return txo.PayloadBytes()
}

func (ec *TxExecutionContext) GasAvailable() uint64 {
	return ec.remainingGas
}

func (ec *TxExecutionContext) SpendGas(gas uint64) error {
	if gas > ec.remainingGas {
		ec.remainingGas = 0
		return ErrOutOfGas
	}
	ec.remainingGas -= gas
	return nil
}

func (ec *TxExecutionContext) CalculateCost() uint64 {
	gasUsed := ec.initialGas - ec.remainingGas
	cost := (gasUsed + GasUnitsPerTema/2) / GasUnitsPerTema
	if cost == 0 {
		cost = 1
	}
	return cost
}

func NewExecutionContext(txSys StateInfo, ts types.RootTrustBase, maxCost uint64) *TxExecutionContext {
	gasUnits := maxCost * GasUnitsPerTema
	return &TxExecutionContext{
		txs:          txSys,
		trustStore:   ts,
		initialGas:   gasUnits,
		remainingGas: gasUnits,
	}
}
