package tokens

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

type (
	Options struct {
		moneyPartitionID     types.PartitionID
		hashAlgorithm        gocrypto.Hash
		trustBase            types.RootTrustBase
		state                *state.State
		executedTransactions map[string]uint64
		exec                 predicates.PredicateExecutor
		adminOwnerPredicate  []byte
		feelessMode          bool
	}

	Option func(*Options)
)

func defaultOptions(observe txsystem.Observability) (*Options, error) {
	templEng, err := templates.New(observe)
	if err != nil {
		return nil, fmt.Errorf("creating predicate template executor: %w", err)
	}
	predEng, err := predicates.Dispatcher(templEng)
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		moneyPartitionID: money.DefaultPartitionID,
		hashAlgorithm:    gocrypto.SHA256,
		exec:             predEng.Execute,
	}, nil
}

func WithState(s *state.State) Option {
	return func(c *Options) {
		c.state = s
	}
}

func WithExecutedTransactions(executedTransactions map[string]uint64) Option {
	return func(c *Options) {
		c.executedTransactions = executedTransactions
	}
}

func WithMoneyPartitionID(moneyPartitionID types.PartitionID) Option {
	return func(c *Options) {
		c.moneyPartitionID = moneyPartitionID
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
