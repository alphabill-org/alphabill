package orchestration

import (
	"crypto"
	"errors"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
)

var _ txtypes.Module = (*Module)(nil)

type (
	Module struct {
		state          *state.State
		systemID       types.SystemID
		ownerPredicate types.PredicateBytes
		hashAlgorithm  crypto.Hash
		execPredicate  predicates.PredicateRunner
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
		execPredicate:  predicates.NewPredicateRunner(options.exec),
	}
	return m, nil
}

func (m *Module) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		orchestration.PayloadTypeAddVAR: txtypes.NewTxHandler[orchestration.AddVarAttributes](m.validateAddVarTx, m.executeAddVarTx),
	}
}
