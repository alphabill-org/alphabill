package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	TxExecutors map[string]TxExecutor

	TxExecutor interface {
		ExecuteFunc() ExecuteFunc
	}

	ExecuteFunc func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error)

	GenericExecuteFunc[T any] func(tx *types.TransactionOrder, attributes *T, currentBlockNr uint64) (*types.ServerMetadata, error)
)

func (g GenericExecuteFunc[T]) ExecuteFunc() ExecuteFunc {
	return func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
		p := new(T)
		if err := tx.UnmarshalAttributes(p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		return g(tx, p, currentBlockNr)
	}
}

func (e ExecuteFunc) ExecuteFunc() ExecuteFunc {
	return e
}

func (e TxExecutors) Execute(g *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
	executor, found := e[g.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", g.PayloadType())
	}

	f := executor.ExecuteFunc()
	sm, err := f(g, currentBlockNr)
	if err != nil {
		return nil, fmt.Errorf("tx order execution failed: %w", err)
	}
	return sm, nil
}
