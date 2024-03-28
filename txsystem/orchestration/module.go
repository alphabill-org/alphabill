package orchestration

import (
	"crypto"
	"errors"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

var _ txsystem.Module = (*Module)(nil)

type (
	Module struct {
		state          *state.State
		systemID       types.SystemID
		ownerPredicate types.PredicateBytes
		hashAlgorithm  crypto.Hash
		execPredicate  func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
	}
)

func NewModule(options *Options) (*Module, error) {
	if options == nil {
		return nil, errors.New("money module options are missing")
	}
	if options.state == nil {
		return nil, errors.New("state is nil")
	}
	if options.ownerPredicate == nil {
		return nil, errors.New("owner predicate is nil")
	}
	m := &Module{
		state:          options.state,
		systemID:       options.systemIdentifier,
		ownerPredicate: options.ownerPredicate,
		hashAlgorithm:  options.hashAlgorithm,
		execPredicate:  predicates.PredicateRunner(options.exec, options.state),
	}
	return m, nil
}

func (m *Module) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeAddVAR: m.handleAddVarTx().ExecuteFunc(),
	}
}
