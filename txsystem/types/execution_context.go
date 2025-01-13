package types

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type (
	StateInfo interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		CurrentRound() uint64
	}

	FeeCalculation interface {
		BuyGas(tema uint64) uint64
		CalculateCost(spentGas uint64) uint64
	}

	// TxExecutionContext - implementation of ExecutionContext interface for generic tx handler
	TxExecutionContext struct {
		txs          StateInfo
		fee          FeeCalculation
		trustStore   types.RootTrustBase
		initialGas   uint64
		remainingGas uint64
		customData   []byte
		exArgument   func() ([]byte, error)
	}
)

func (ec *TxExecutionContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
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

/*
ExArgument calls the function set using WithExArg method.

This can be used to provide "extra argument" for the predicate, currently used
ie by the P2PKH predicate to receive the signature bytes it should verify.
*/
func (ec *TxExecutionContext) ExArgument() ([]byte, error) {
	if ec.exArgument == nil {
		return nil, errors.New("extra argument callback not assigned")
	}
	return ec.exArgument()
}

func (ec *TxExecutionContext) GetData() []byte {
	return ec.customData
}

func (ec *TxExecutionContext) SetData(data []byte) {
	ec.customData = data
}

/*
WithExArg sets the "extra argument" callback which is used by the ExArgument method.
*/
func (ec *TxExecutionContext) WithExArg(f func() ([]byte, error)) ExecutionContext {
	ec.exArgument = f
	return ec
}

func NewExecutionContext(txSys StateInfo, f FeeCalculation, tb types.RootTrustBase, maxCost uint64) *TxExecutionContext {
	gasUnits := f.BuyGas(maxCost)
	return &TxExecutionContext{
		txs:          txSys,
		fee:          f,
		trustStore:   tb,
		initialGas:   gasUnits,
		remainingGas: gasUnits,
	}
}
