package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	txExecutors map[string]txsystem.TxExecutor
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{
		txExecutors: map[string]txsystem.TxExecutor{
			PayloadTypeLockToken:   handleLockTokenTx(options),
			PayloadTypeUnlockToken: handleUnlockTokenTx(options),
		},
	}, nil
}

func (n *LockTokensModule) TxExecutors() map[string]txsystem.TxExecutor {
	return n.txExecutors
}

func (n *LockTokensModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return ValidateGenericTransaction
}
