package tokens

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
)

type (
	Options struct {
		moneySystemID       types.SystemID
		hashAlgorithm       gocrypto.Hash
		trustBase           types.RootTrustBase
		state               *state.State
		exec                predicates.PredicateExecutor
		adminOwnerPredicate []byte
		feelessMode         bool
	}

	Option func(*Options)
)

func defaultOptions() (*Options, error) {
	predEng, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		moneySystemID: money.DefaultSystemID,
		hashAlgorithm: gocrypto.SHA256,
		exec:          predEng.Execute,
	}, nil
}

func WithState(s *state.State) Option {
	return func(c *Options) {
		c.state = s
	}
}

func WithMoneyTransactionSystemID(moneySystemID types.SystemID) Option {
	return func(c *Options) {
		c.moneySystemID = moneySystemID
	}
}

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(trustBase types.RootTrustBase) Option {
	return func(c *Options) {
		c.trustBase = trustBase
	}
}

func WithAdminOwnerPredicate(adminOwnerPredicate []byte) Option {
	return func(c *Options) {
		c.adminOwnerPredicate = adminOwnerPredicate
	}
}

func WithFeelessMode(feelessMode bool) Option {
	return func(c *Options) {
		c.feelessMode = feelessMode
	}
}

/*
WithPredicateExecutor allows to replace the default predicate executor which
supports only "builtin predicate templates".
*/
func WithPredicateExecutor(exec predicates.PredicateExecutor) Option {
	return func(g *Options) {
		if exec != nil {
			g.exec = exec
		}
	}
}
