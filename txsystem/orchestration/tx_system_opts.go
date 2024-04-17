package orchestration

import (
	"context"
	"crypto"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill-go-sdk/types"
)

const DefaultSystemIdentifier types.SystemID = 4

type (
	Options struct {
		systemIdentifier types.SystemID
		state            *state.State
		hashAlgorithm    crypto.Hash
		ownerPredicate   types.PredicateBytes
		trustBase        map[string]abcrypto.Verifier
		exec             PredicateExecutor
	}

	Option func(*Options)

	PredicateExecutor func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error)
)

func defaultOptions() (*Options, error) {
	predEng, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		systemIdentifier: DefaultSystemIdentifier,
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

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithTrustBase(tb map[string]abcrypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = tb
	}
}

func WithOwnerPredicate(ownerPredicate types.PredicateBytes) Option {
	return func(g *Options) {
		g.ownerPredicate = ownerPredicate
	}
}
