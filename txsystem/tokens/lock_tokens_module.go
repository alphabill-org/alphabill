package tokens

import "github.com/alphabill-org/alphabill/txsystem"

var _ txsystem.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	options *Options
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{options: options}, nil
}

func (n *LockTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeLockToken:   handleLockTokenTx(n.options).ExecuteFunc(),
		PayloadTypeUnlockToken: handleUnlockTokenTx(n.options).ExecuteFunc(),
	}
}
