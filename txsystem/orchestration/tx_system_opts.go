package orchestration

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

type (
	Options struct {
		state          *state.State
		hashAlgorithm  crypto.Hash
		ownerPredicate types.PredicateBytes
		trustBase      types.RootTrustBase
		exec           predicates.PredicateExecutor
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
		hashAlgorithm: crypto.SHA256,
		exec:          predEng.Execute,
	}, nil
}

func WithState(s *state.State) Option {
	return func(g *Options) {
		g.state = s
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithTrustBase(trustBase types.RootTrustBase) Option {
	return func(c *Options) {
		c.trustBase = trustBase
	}
}

func WithOwnerPredicate(ownerPredicate types.PredicateBytes) Option {
	return func(g *Options) {
		g.ownerPredicate = ownerPredicate
	}
}
