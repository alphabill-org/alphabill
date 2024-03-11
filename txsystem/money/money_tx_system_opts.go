package money

import (
	"context"
	"crypto"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/types"
)

const DefaultSystemIdentifier types.SystemID = 0x00000001

type (
	Options struct {
		systemIdentifier         types.SystemID
		state                    *state.State
		hashAlgorithm            crypto.Hash
		trustBase                map[string]abcrypto.Verifier
		systemDescriptionRecords []*genesis.SystemDescriptionRecord
		feeCalculator            fc.FeeCalculator
		exec                     PredicateExecutor
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
		trustBase:        make(map[string]abcrypto.Verifier),
		feeCalculator:    fc.FixedFee(1),
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

func WithTrustBase(trust map[string]abcrypto.Verifier) Option {
	return func(options *Options) {
		options.trustBase = trust
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithSystemDescriptionRecords(records []*genesis.SystemDescriptionRecord) Option {
	return func(g *Options) {
		g.systemDescriptionRecords = records
	}
}

func WithFeeCalculator(calc fc.FeeCalculator) Option {
	return func(g *Options) {
		g.feeCalculator = calc
	}
}

/*
WithPredicateExecutor allows to replace the default predicate executor function.
Should be used by tests only.
*/
func WithPredicateExecutor(exec PredicateExecutor) Option {
	return func(g *Options) {
		if exec != nil {
			g.exec = exec
		}
	}
}
