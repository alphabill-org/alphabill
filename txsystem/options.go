package txsystem

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	abfc "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

type Options struct {
	hashAlgorithm       crypto.Hash
	state               *state.State
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
	predicateRunner     predicates.PredicateRunner
	feeCredit           txtypes.FeeCreditModule
	observe             Observability
}

type Option func(*Options) error

func DefaultOptions(observe Observability) (*Options, error) {
	return (&Options{
		hashAlgorithm: crypto.SHA256,
		state:         state.NewEmptyState(),
		feeCredit:     abfc.NewNoFeeCreditModule(),
		observe:       observe,
	}).initPredicateRunner(observe)
}

func WithBeginBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) error {
		g.beginBlockFunctions = append(g.beginBlockFunctions, funcs...)
		return nil
	}
}

func WithEndBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) error {
		g.endBlockFunctions = append(g.endBlockFunctions, funcs...)
		return nil
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) error {
		g.hashAlgorithm = hashAlgorithm
		return nil
	}
}

func WithState(s *state.State) Option {
	return func(g *Options) error {
		g.state = s
		// re-init predicate runner
		g.initPredicateRunner(g.observe)
		return nil
	}
}

func WithFeeCredits(f txtypes.FeeCreditModule) Option {
	return func(g *Options) error {
		g.feeCredit = f
		return nil
	}
}

func (o *Options) initPredicateRunner(observe Observability) (*Options, error) {
	templEng, err := templates.New(observe)
	if err != nil {
		return nil, fmt.Errorf("creating predicate template executor: %w", err)
	}
	engines, err := predicates.Dispatcher(templEng)
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}
	o.predicateRunner = predicates.NewPredicateRunner(engines.Execute)
	return o, nil
}
