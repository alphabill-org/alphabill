package tokens

import (
	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
)

var _ txsystem2.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	txExecutors map[string]txsystem2.TxExecutor
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{
		txExecutors: map[string]txsystem2.TxExecutor{
			PayloadTypeLockToken:   handleLockTokenTx(options),
			PayloadTypeUnlockToken: handleUnlockTokenTx(options),
		},
	}, nil
}

func (n *LockTokensModule) TxExecutors() map[string]txsystem2.TxExecutor {
	return n.txExecutors
}

func (n *LockTokensModule) GenericTransactionValidator() txsystem2.GenericTransactionValidator {
	return ValidateGenericTransaction
}
