package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

var _ txsystem.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	state         *state.State
	feeCalculator fc.FeeCalculator
	hashAlgorithm crypto.Hash
	execPredicate func(predicate, args []byte, txo *types.TransactionOrder) error
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{
		state:         options.state,
		feeCalculator: options.feeCalculator,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: PredicateRunner(options.exec, options.state),
	}, nil
}

func (n *LockTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		tokens.PayloadTypeLockToken:   n.handleLockTokenTx().ExecuteFunc(),
		tokens.PayloadTypeUnlockToken: n.handleUnlockTokenTx().ExecuteFunc(),
	}
}
