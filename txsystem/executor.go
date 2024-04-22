package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"
)

type (
	TxExecutors map[string]ExecuteFunc

	ExecuteFunc func(*types.TransactionOrder, *TxExecutionContext) (*types.ServerMetadata, error)

	GenericExecuteFunc[T any] func(tx *types.TransactionOrder, attributes *T, exeCtx *TxExecutionContext) (*types.ServerMetadata, error)

	TxExecutionContext struct {
		CurrentBlockNr    uint64
		StateLockReleased bool // if true, the tx being executed was "on hold" and must use this flag to avoid locking the state again
	}
)

func (g GenericExecuteFunc[T]) ExecuteFunc() ExecuteFunc {
	return func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
		attr := new(T)
		if err := tx.UnmarshalAttributes(attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		return g(tx, attr, exeCtx)
	}
}

func (e TxExecutors) Execute(txo *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
	executor, found := e[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}

	sm, err := executor(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("tx order execution failed: %w", err)
	}
	return sm, nil
}

func (e TxExecutors) Add(src TxExecutors) error {
	for name, handler := range src {
		if name == "" {
			return fmt.Errorf("tx executor must have non-empty tx type name")
		}
		if handler == nil {
			return fmt.Errorf("tx executor must not be nil (%s)", name)
		}
		if _, ok := e[name]; ok {
			return fmt.Errorf("tx executor for %q is already registered", name)
		}
		e[name] = handler
	}
	return nil
}
