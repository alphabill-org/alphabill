package txsystem

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type (
	StateInfo interface {
		State() StateReader
		CurrentRound() uint64
	}

	// TxExecutionContext - implementation of ExecutionContext interface for generic tx handler
	TxExecutionContext struct {
		txs       StateInfo
		trustBase types.RootTrustBase
	}
)

func newExecutionContext(txSys StateInfo, tb types.RootTrustBase) *TxExecutionContext {
	return &TxExecutionContext{
		txs:       txSys,
		trustBase: tb,
	}
}

func (ec TxExecutionContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return ec.txs.State().GetUnit(id, committed)
}

func (ec TxExecutionContext) CurrentRound() uint64 { return ec.txs.CurrentRound() }

func (ec TxExecutionContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return ec.trustBase, nil
}

// until AB-1012 gets resolved we need this hack to get correct payload bytes.
func (ec TxExecutionContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return txo.PayloadBytes()
}
