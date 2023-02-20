package txsystem

import (
	"fmt"
	"reflect"
)

type (
	TxExecutors map[reflect.Type]ExecuteFunc

	TxExecutor interface {
		Type() reflect.Type
		ExecuteFunc() ExecuteFunc
	}

	ExecuteFunc func(tx GenericTransaction, currentBlockNr uint64) error

	GenericExecuteFunc[T GenericTransaction] func(t T, currentBlockNr uint64) error
)

func (g GenericExecuteFunc[T]) Type() reflect.Type {
	return reflect.TypeOf(*new(T))
}

func (g GenericExecuteFunc[T]) ExecuteFunc() ExecuteFunc {
	return func(tx GenericTransaction, currentBlockNr uint64) error {
		return g(tx.(T), currentBlockNr)
	}
}

func (e TxExecutors) Execute(g GenericTransaction, currentBlockNr uint64) error {
	txType := reflect.TypeOf(g)
	executor, found := e[txType]
	if !found {
		return fmt.Errorf("unknown transaction type %T", g)
	}
	err := executor(g, currentBlockNr)
	if err != nil {
		return fmt.Errorf("tx of type %T execution failed: %w", g, err)
	}
	return nil
}
