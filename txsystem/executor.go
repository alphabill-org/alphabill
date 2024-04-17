package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"
)

type (
	TxExecutors map[string]ExecuteFunc

	ExecuteFunc func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error)

	GenericExecuteFunc[T any] func(tx *types.TransactionOrder, attributes *T, currentBlockNr uint64) (*types.ServerMetadata, error)
)

func (g GenericExecuteFunc[T]) ExecuteFunc() ExecuteFunc {
	return func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
		attr := new(T)
		if err := tx.UnmarshalAttributes(attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		return g(tx, attr, currentBlockNr)
	}
}

func (e TxExecutors) Execute(txo *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
	executor, found := e[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}

	sm, err := executor(txo, currentBlockNr)
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
