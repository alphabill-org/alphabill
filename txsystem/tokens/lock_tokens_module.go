package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.Module = (*LockTokensModule)(nil)

type LockTokensModule struct {
	state         *state.State
	hashAlgorithm crypto.Hash
	execPredicate predicates.PredicateRunner
}

func NewLockTokensModule(options *Options) (*LockTokensModule, error) {
	return &LockTokensModule{
		state:         options.state,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: predicates.NewPredicateRunner(options.exec),
	}, nil
}

func (m *LockTokensModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		tokens.PayloadTypeLockToken:   txtypes.NewTxHandler[tokens.LockTokenAttributes, tokens.LockTokenAuthProof](m.validateLockTokenTx, m.executeLockTokensTx),
		tokens.PayloadTypeUnlockToken: txtypes.NewTxHandler[tokens.UnlockTokenAttributes, tokens.UnlockTokenAuthProof](m.validateUnlockTokenTx, m.executeUnlockTokenTx),
	}
}
