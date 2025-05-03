package types

import (
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

// transaction execution types
const (
	ExecutionTypeUnconditional ExecutionType = iota // normal execution
	ExecutionTypeConditional                        // creating dummy units and placing tx "on hold"
	ExecutionTypeCommit                             // converting dummy units to normal units and executing "on hold" tx
	ExecutionTypeRollback                           // deleting dummy units?
)

type (
	StateInfo interface {
		GetUnit(id types.UnitID, committed bool) (state.Unit, error)
		CommittedUC() *types.UnicityCertificate
		CurrentRound() uint64
	}

	FeeCalculation interface {
		BuyGas(tema uint64) uint64
		CalculateCost(spentGas uint64) uint64
	}

	// TxExecutionContext - implementation of ExecutionContext interface for generic tx handler
	TxExecutionContext struct {
		txs           StateInfo
		fee           FeeCalculation
		initialGas    uint64
		remainingGas  uint64
		customData    []byte
		exArgument    func() ([]byte, error)
		executionType ExecutionType
	}

	ExecutionType uint
)

func NewExecutionContext(txSys StateInfo, f FeeCalculation, maxCost uint64) *TxExecutionContext {
	gasUnits := f.BuyGas(maxCost)
	return &TxExecutionContext{
		txs:          txSys,
		fee:          f,
		initialGas:   gasUnits,
		remainingGas: gasUnits,
	}
}

func (ec *TxExecutionContext) GetUnit(id types.UnitID, committed bool) (state.Unit, error) {
	return ec.txs.GetUnit(id, committed)
}

func (ec *TxExecutionContext) CommittedUC() *types.UnicityCertificate {
	return ec.txs.CommittedUC()
}

func (ec *TxExecutionContext) CurrentRound() uint64 { return ec.txs.CurrentRound() }

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
ExtraArgument calls the function set using WithExArg method.

This can be used to provide "extra argument" for the predicate, currently used
ie by the P2PKH predicate to receive the signature bytes it should verify.
*/
func (ec *TxExecutionContext) ExtraArgument() ([]byte, error) {
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
WithExArg sets the "extra argument" callback which is used by the ExtraArgument method.
*/
func (ec *TxExecutionContext) WithExArg(f func() ([]byte, error)) ExecutionContext {
	ec.exArgument = f
	return ec
}

func (ec *TxExecutionContext) ExecutionType() ExecutionType {
	return ec.executionType
}

func (ec *TxExecutionContext) SetExecutionType(exeType ExecutionType) {
	ec.executionType = exeType
}
