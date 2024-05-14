package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
)

type (
	TxHandler[T any] struct {
		Execute  func(tx *types.TransactionOrder, attributes *T, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		Validate func(tx *types.TransactionOrder, attributes *T, exeCtx ExecutionContext) error
	}

	TxExecutor interface {
		ValidateTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (any, error)
		ExecuteTxWithAttr(tx *types.TransactionOrder, attributes any, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		ExecuteTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error)
	}

	TxExecutors map[string]TxExecutor

	ExecuteFunc func(*types.TransactionOrder, ExecutionContext) (*types.ServerMetadata, error)

	ValidateFunc func(*types.TransactionOrder, ExecutionContext) error

	GenericExecuteFunc[T any] func(tx *types.TransactionOrder, attributes *T, exeCtx ExecutionContext) (*types.ServerMetadata, error)

	GenericValidateFunc[T any] func(tx *types.TransactionOrder, attributes *T, exeCtx ExecutionContext) error

	// ExecutionContext - provides additional context and info for tx validation and execution
	ExecutionContext interface {
		predicates.TxContext
	}
)

func NewTxHandler[T any](v GenericValidateFunc[T], e GenericExecuteFunc[T]) *TxHandler[T] {
	return &TxHandler[T]{Validate: v, Execute: e}
}

func (t *TxHandler[T]) ValidateTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, error) {
	attr := new(T)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	if err := t.Validate(txo, attr, exeCtx); err != nil {
		return nil, err
	}
	return attr, nil
}

func (t *TxHandler[T]) ExecuteTxWithAttr(txo *types.TransactionOrder, attr any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	txAttr, ok := attr.(*T)
	if !ok {
		return nil, fmt.Errorf("incorrect attribute type: %T for tx order %s", attr, txo.PayloadType())
	}
	return t.Execute(txo, txAttr, exeCtx)
}

func (t *TxHandler[T]) ExecuteTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	attr := new(T)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return t.Execute(txo, attr, exeCtx)
}

func (h TxExecutors) ValidateAndExecute(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}
	attr, err := handler.ValidateTx(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("'%s' validation failed: %w", txo.PayloadType(), err)
	}
	sm, err := handler.ExecuteTxWithAttr(txo, attr, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("'%s' execution failed: %w", txo.PayloadType(), err)
	}
	return sm, nil
}

func (h TxExecutors) Validate(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}

	return handler.ValidateTx(txo, exeCtx)
}

func (h TxExecutors) ExecuteWithAttr(txo *types.TransactionOrder, attr any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}

	return handler.ExecuteTxWithAttr(txo, attr, exeCtx)
}

func (h TxExecutors) Execute(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}

	sm, err := handler.ExecuteTx(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("tx order execution failed: %w", err)
	}
	return sm, nil
}

func (h TxExecutors) Add(src TxExecutors) error {
	for name, handler := range src {
		if name == "" {
			return fmt.Errorf("tx executor must have non-empty tx type name")
		}
		if handler == nil {
			return fmt.Errorf("tx executor must not be nil (%s)", name)
		}
		if _, ok := h[name]; ok {
			return fmt.Errorf("tx executor for %q is already registered", name)
		}
		h[name] = handler
	}
	return nil
}
