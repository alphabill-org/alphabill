package types

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
)

type (
	Module interface {
		TxHandlers() map[uint16]TxExecutor
	}

	TxHandler[A any, P any] struct {
		Execute     func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		Validate    func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) error
		TargetUnits func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) ([]types.UnitID, error)
	}

	TxExecutor interface {
		UnmarshalTx(txo *types.TransactionOrder, ctx ExecutionContext) (any, any, []types.UnitID, error)
		ValidateTx(tx *types.TransactionOrder, attributes any, authProof any, exeCtx ExecutionContext) error
		ExecuteTxWithAttr(tx *types.TransactionOrder, attributes any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error)
		ExecuteTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error)
	}

	TxExecutors map[uint16]TxExecutor

	ExecuteFunc func(*types.TransactionOrder, ExecutionContext) (*types.ServerMetadata, error)

	ValidateFunc func(*types.TransactionOrder, ExecutionContext) error

	GenericExecuteFunc[A, P any] func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) (*types.ServerMetadata, error)

	GenericValidateFunc[A, P any] func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) error

	GenericTargetUnitsFunc[A, P any] func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) ([]types.UnitID, error)

	// ExecutionContext - provides additional context and info for tx validation and execution
	ExecutionContext interface {
		predicates.TxContext
		GetData() []byte     // read data added by the GetData function
		SetData(data []byte) // add arbitrary data to the execution context
		WithExArg(func() ([]byte, error)) ExecutionContext
		ExecutionType() ExecutionType
		SetExecutionType(exeType ExecutionType)
	}

	TxHandlerOption[A any, P any] func(*TxHandler[A, P])
)

func NewTxHandler[A, P any](v GenericValidateFunc[A, P], e GenericExecuteFunc[A, P], opts ...TxHandlerOption[A, P]) *TxHandler[A, P] {
	defaultTargetUnitsFn := func(tx *types.TransactionOrder, attributes *A, authProof *P, exeCtx ExecutionContext) ([]types.UnitID, error) {
		return []types.UnitID{tx.UnitID}, nil
	}
	handler := &TxHandler[A, P]{
		Validate:    v,
		Execute:     e,
		TargetUnits: defaultTargetUnitsFn,
	}
	for _, opt := range opts {
		opt(handler)
	}
	return handler
}

func WithTargetUnitsFn[A, P any](u GenericTargetUnitsFunc[A, P]) TxHandlerOption[A, P] {
	return func(handler *TxHandler[A, P]) {
		handler.TargetUnits = u
	}
}

func (t *TxHandler[A, P]) UnmarshalTx(txo *types.TransactionOrder, exeCtx ExecutionContext) (any, any, []types.UnitID, error) {
	attr := new(A)
	if err := txo.UnmarshalAttributes(attr); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	authProof := new(P)
	if err := txo.UnmarshalAuthProof(authProof); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal auth proof: %w", err)
	}
	targetUnits, err := t.TargetUnits(txo, attr, authProof, exeCtx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal target units: %w", err)
	}
	return attr, authProof, targetUnits, nil
}

func (t *TxHandler[A, P]) ValidateTx(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) error {
	// cannot directly accept generic params in order to satisfy types.TxExecutor interface
	txAttr, ok := attr.(*A)
	if !ok {
		return fmt.Errorf("incorrect attribute type: %T for transaction order %d", attr, txo.Type)
	}
	txAuthProof, ok := authProof.(*P)
	if !ok {
		return fmt.Errorf("incorrect auth proof type: %T for transaction order %d", authProof, txo.Type)
	}
	return t.Validate(txo, txAttr, txAuthProof, exeCtx)
}

func (t *TxHandler[A, P]) ExecuteTxWithAttr(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	txAttr, ok := attr.(*A)
	if !ok {
		return nil, fmt.Errorf("incorrect attribute type: %T for transaction order %d", attr, txo.Type)
	}
	txAuthProof, ok := authProof.(*P)
	if !ok {
		return nil, fmt.Errorf("incorrect auth proof type: %T for transaction order %d", authProof, txo.Type)
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
	handler, found := h[txo.Type]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %d", txo.Type)
	}
	attr, authProof, _, err := handler.UnmarshalTx(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx with type %d: %w", txo.Type, err)
	}
	if err := handler.ValidateTx(txo, attr, authProof, exeCtx); err != nil {
		return nil, fmt.Errorf("transaction validation failed (type=%d): %w", txo.Type, err)
	}
	sm, err := handler.ExecuteTxWithAttr(txo, attr, authProof, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("transaction execution failed (type=%d): %w", txo.Type, err)
	}
	return sm, nil
}

func (h TxExecutors) UnmarshalTx(tx *types.TransactionOrder, exeCtx ExecutionContext) (any, any, []types.UnitID, error) {
	handler, found := h[tx.Type]
	if !found {
		return nil, nil, nil, fmt.Errorf("unknown transaction type %d", tx.Type)
	}
	return handler.UnmarshalTx(tx, exeCtx)
}

func (h TxExecutors) Validate(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) error {
	handler, found := h[txo.Type]
	if !found {
		return fmt.Errorf("unknown transaction type %d", txo.Type)
	}
	return handler.ValidateTx(txo, attr, authProof, exeCtx)
}

func (h TxExecutors) ExecuteWithAttr(txo *types.TransactionOrder, attr any, authProof any, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.Type]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %d", txo.Type)
	}
	return handler.ExecuteTxWithAttr(txo, attr, authProof, exeCtx)
}

func (h TxExecutors) Execute(txo *types.TransactionOrder, exeCtx ExecutionContext) (*types.ServerMetadata, error) {
	handler, found := h[txo.Type]
	if !found {
		return nil, fmt.Errorf("unknown transaction type %d", txo.Type)
	}

	sm, err := handler.ExecuteTx(txo, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("transaction order execution failed: %w", err)
	}
	return sm, nil
}

func (h TxExecutors) Add(src TxExecutors) error {
	for txType, handler := range src {
		if txType == 0 {
			return fmt.Errorf("transaction executor must have non-zero transaction type")
		}
		if handler == nil {
			return fmt.Errorf("transaction executor must not be nil (type=%d)", txType)
		}
		if _, ok := h[txType]; ok {
			return fmt.Errorf("transaction executor for type=%d is already registered", txType)
		}
		h[txType] = handler
	}
	return nil
}
