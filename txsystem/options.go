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
}

type Option func(*Options)

func DefaultOptions() *Options {
	return (&Options{
		hashAlgorithm: crypto.SHA256,
		state:         state.NewEmptyState(),
		feeCredit:     abfc.NewNoFeeCreditModule(),
	}).initPredicateRunner()
}

func WithBeginBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) {
		g.beginBlockFunctions = append(g.beginBlockFunctions, funcs...)
	}
}

func WithEndBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) {
		g.endBlockFunctions = append(g.endBlockFunctions, funcs...)
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithState(s *state.State) Option {
	return func(g *Options) {
		g.state = s
		// re-init predicate runner
		g.initPredicateRunner()
	}
}

func WithFeeCredits(f txtypes.FeeCreditModule) Option {
	return func(g *Options) {
		g.feeCredit = f
	}
}

func (o *Options) initPredicateRunner() *Options {
	engines, err := predicates.Dispatcher(templates.New())
	if err != nil {
		panic(fmt.Errorf("creating predicate executor: %w", err))
	}
	o.predicateRunner = predicates.NewPredicateRunner(engines.Execute)
	return o
}
