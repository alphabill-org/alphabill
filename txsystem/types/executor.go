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

	TxHandler[A any, P any] struct {
		Execute  func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		Validate func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) error
	}

	TxExecutor interface {
		ValidateTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (any, any, error)
		ExecuteTxWithAttr(tx *types.TransactionOrder, attributes any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		ExecuteTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error)
	}

	TxExecutors map[string]TxExecutor

	ExecuteFunc func(*types.TransactionOrder, ExecutionContext) (*types.ServerMetadata, error)

	ValidateFunc func(*types.TransactionOrder, ExecutionContext) error

	GenericExecuteFunc[A, P any] func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) (*types.ServerMetadata, error)

	GenericValidateFunc[A, P any] func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) error

	// ExecutionContext - provides additional context and info for tx validation and execution
	ExecutionContext interface {
		predicates.TxContext
	}
)

func NewTxHandler[A, P any](v GenericValidateFunc[A, P], e GenericExecuteFunc[A, P]) *TxHandler[A, P] {
	return &TxHandler[A, P]{Validate: v, Execute: e}
}

func (t *TxHandler[A, P]) ValidateTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, any, error) {
	attr := new(A)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	authProof := new(P)
	if err := txo.UnmarshalAuthProof(authProof); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal auth proof: %w", err)
	}
	if err := t.Validate(txo, attr, authProof, exeCtx); err != nil {
		return nil, nil, err
	}
	return attr, authProof, nil
}

func (t *TxHandler[A, P]) ExecuteTxWithAttr(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	txAttr, ok := attr.(*A)
	if !ok {
		return nil, fmt.Errorf("incorrect attribute type: %T for transaction order %s", attr, txo.PayloadType())
	}
	txAuthProof, ok := authProof.(*P)
	if !ok {
		return nil, fmt.Errorf("incorrect auth proof type: %T for transaction order %s", authProof, txo.PayloadType())
	}
	return t.Execute(txo, txAttr, txAuthProof, exeCtx)
}

func (t *TxHandler[A, P]) ExecuteTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	attr := new(A)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	authProof := new(P)
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
		return nil, fmt.Errorf("transaction order execution failed: %w", err)
	}
	return sm, nil
}

func (h TxExecutors) Add(src TxExecutors) error {
	for name, handler := range src {
		if name == "" {
			return fmt.Errorf("transaction executor must have non-empty transaction type name")
		}
		if handler == nil {
			return fmt.Errorf("transaction executor must not be nil (%s)", name)
		}
		if _, ok := h[name]; ok {
			return fmt.Errorf("transaction executor for %q is already registered", name)
		}
		h[name] = handler
	}
	return nil
}
