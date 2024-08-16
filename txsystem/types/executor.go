package types

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
)

type (
	Module interface {
		TxHandlers() map[string]TxExecutor
	}

	TxHandler[T any, T1 any] struct {
		Execute  func(tx *types.TransactionOrder, attributes *T, authProof *T1, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		Validate func(tx *types.TransactionOrder, attributes *T, authProof *T1, exeCtx ExecutionContext) error
	}

	TxExecutor interface {
		ValidateTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (any, any, error)
		ExecuteTxWithAttr(tx *types.TransactionOrder, attributes any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		ExecuteTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error)
	}

	TxExecutors map[string]TxExecutor

	ExecuteFunc func(*types.TransactionOrder, ExecutionContext) (*types.ServerMetadata, error)

	ValidateFunc func(*types.TransactionOrder, ExecutionContext) error

	GenericExecuteFunc[T, T1 any] func(tx *types.TransactionOrder, attributes *T, authProof *T1, exeCtx ExecutionContext) (*types.ServerMetadata, error)

	GenericValidateFunc[T, T1 any] func(tx *types.TransactionOrder, attributes *T, authProof *T1, exeCtx ExecutionContext) error

	// ExecutionContext - provides additional context and info for tx validation and execution
	ExecutionContext interface {
		predicates.TxContext
	}
)

func NewTxHandler[T, T1 any](v GenericValidateFunc[T, T1], e GenericExecuteFunc[T, T1]) *TxHandler[T, T1] {
	return &TxHandler[T, T1]{Validate: v, Execute: e}
}

func (t *TxHandler[T, T1]) ValidateTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, any, error) {
	attr := new(T)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	authProof := new(T1)
	if err := txo.UnmarshalAuthProof(authProof); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal auth proof: %w", err)
	}
	if err := t.Validate(txo, attr, authProof, exeCtx); err != nil {
		return nil, nil, err
	}
	return attr, authProof, nil
}

func (t *TxHandler[T, T1]) ExecuteTxWithAttr(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	txAttr, ok := attr.(*T)
	if !ok {
		return nil, fmt.Errorf("incorrect attribute type: %T for tx order %s", attr, txo.PayloadType())
	}
	txAuthProof, ok := authProof.(*T1)
	if !ok {
		return nil, fmt.Errorf("incorrect auth proof type: %T for tx order %s", attr, txo.PayloadType())
	}
	return t.Execute(txo, txAttr, txAuthProof, exeCtx)
}

func (t *TxHandler[T, T1]) ExecuteTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	attr := new(T)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	authProof := new(T1)
	if err := txo.UnmarshalAuthProof(authProof); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auth proof: %w", err)
	}
	return t.Execute(txo, attr, authProof, exeCtx)
}

func (h TxExecutors) ValidateAndExecute(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}
	attr, authProof, err := handler.ValidateTx(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("'%s' validation failed: %w", txo.PayloadType(), err)
	}
	sm, err := handler.ExecuteTxWithAttr(txo, attr, authProof, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("'%s' execution failed: %w", txo.PayloadType(), err)
	}
	return sm, nil
}

func (h TxExecutors) Validate(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, any, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}
	return handler.ValidateTx(txo, exeCtx)
}

func (h TxExecutors) ExecuteWithAttr(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.PayloadType()]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %s", txo.PayloadType())
	}
	return handler.ExecuteTxWithAttr(txo, attr, authProof, exeCtx)
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
