package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

var _ txsystem.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	state         *state.State
	feeCalculator fc.FeeCalculator
	hashAlgorithm crypto.Hash
	execPredicate predicates.PredicateRunner
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{
		state:         options.state,
		feeCalculator: options.feeCalculator,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: PredicateRunner(options.exec, options.state),
	}, nil
}

func (m *LockTokensModule) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		tokens.PayloadTypeLockToken:   txsystem.NewTxHandler[tokens.LockTokenAttributes](m.validateLockTokenTx, m.executeLockTokensTx),
		tokens.PayloadTypeUnlockToken: txsystem.NewTxHandler[tokens.UnlockTokenAttributes](m.validateUnlockTokenTx, m.executeUnlockTokenTx),
	}
}
