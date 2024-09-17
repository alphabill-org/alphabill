package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
)

type (
	Options struct {
		systemIdentifier         types.SystemID
		state                    *state.State
		hashAlgorithm            crypto.Hash
		trustBase                types.RootTrustBase
		systemDescriptionRecords []*types.PartitionDescriptionRecord
		exec                     predicates.PredicateExecutor
	}

	Option func(*Options)
)

func defaultOptions() (*Options, error) {
	predEng, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		systemIdentifier: money.DefaultSystemID,
		hashAlgorithm:    crypto.SHA256,
		exec:             predEng.Execute,
	}, nil
}

func WithSystemIdentifier(systemIdentifier types.SystemID) Option {
	return func(g *Options) {
		g.systemIdentifier = systemIdentifier
	}
}

func WithState(s *state.State) Option {
	return func(g *Options) {
		g.state = s
	}
}

func WithTrustBase(trust types.RootTrustBase) Option {
	return func(options *Options) {
		options.trustBase = trust
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithPartitionDescriptionRecords(records []*types.PartitionDescriptionRecord) Option {
	return func(g *Options) {
		g.systemDescriptionRecords = records
	}
}

/*
WithPredicateExecutor allows to replace the default predicate executor function.
Should be used by tests only.
*/
func WithPredicateExecutor(exec predicates.PredicateExecutor) Option {
	return func(g *Options) {
		if exec != nil {
			g.exec = exec
		}
	}
}
