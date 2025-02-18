package predicates

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

const MaxPredicateBinSize = 65536

type (
	PredicateEngine interface {
		// ID unique ID of the engine, this is used to dispatch predicates (predicate.Tag == engine.ID)
		// to the engine which is supposed to evaluate it.
		ID() uint64

		// Execute executes given predicate
		Execute(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)
	}

	PredicateEngines map[uint64]func(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)

	PredicateExecutor func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)

	PredicateRunner func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) error

	GasMeter interface {
		GasAvailable() uint64
		SpendGas(gas uint64) error
		CalculateCost() uint64
	}

	// TxContext environment where predicate runs (AKA transaction execution context)
	// This is meant to provide the predicate engine with access to the
	// tx system which processes the transaction.
	TxContext interface {
		GasMeter
		GetUnit(id types.UnitID, committed bool) (state.Unit, error)
		CommittedUC() *types.UnicityCertificate
		CurrentRound() uint64
		TrustBase(epoch uint64) (types.RootTrustBase, error)
		ExtraArgument() ([]byte, error)
	}
)

func ExtractPredicate(predicateBytes []byte) (*predicates.Predicate, error) {
	predicate := &predicates.Predicate{}
	if err := types.Cbor.Unmarshal(predicateBytes, predicate); err != nil {
		return nil, err
	}
	return predicate, nil
}

/*
Dispatcher creates collection of predicate engines
*/
func Dispatcher(engines ...PredicateEngine) (PredicateEngines, error) {
	pe := make(PredicateEngines, len(engines))
	for x, v := range engines {
		if err := pe.Add(v); err != nil {
			return nil, fmt.Errorf("registering predicate engine %d of %d: %w", x+1, len(engines), err)
		}
	}
	return pe, nil
}

func (pe PredicateEngines) Add(engine PredicateEngine) error {
	if _, ok := pe[engine.ID()]; ok {
		return fmt.Errorf("predicate engine with id %d is already registered", engine.ID())
	}

	pe[engine.ID()] = engine.Execute

	return nil
}

/*
Execute decodes predicate from binary representation and dispatches it to appropriate predicate executor.
*/
func (pe PredicateEngines) Execute(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
	if len(predicate) == 0 {
		return false, errors.New("predicate is empty")
	}
	if len(predicate) > MaxPredicateBinSize {
		return false, fmt.Errorf("predicate is too large, max allowed is %d got %d bytes", MaxPredicateBinSize, len(predicate))
	}
	pred, err := ExtractPredicate(predicate)
	if err != nil {
		return false, fmt.Errorf("decoding predicate: %w", err)
	}

	executor, ok := pe[pred.Tag]
	if !ok {
		return false, fmt.Errorf("unknown predicate engine with id %d", pred.Tag)
	}
	result, err := executor(ctx, pred, args, txo, env)
	if err != nil {
		return false, fmt.Errorf("executing predicate: %w", err)
	}
	return result, nil
}

/*
NewPredicateRunner is a helper to refactor predicate support - provide implementation which is common
for most tx systems and "translates" between two interfaces:
  - currently tx handlers do not have context.Context to pass to the predicate engine so wrapper
    returned doesn't require it and passes context.Background() to the predicate engine;
  - currently tx systems do not differentiate between predicate evaluating to "false" vs returning
    error (ie invalid predicate or arguments) so wrapper returns error in case the predicate
    evaluates to "false".
*/
func NewPredicateRunner(
	// executor is the function which takes raw predicate binary and routes it to correct predicate engine.
	// usually it is PredicateEngines.Execute
	executor PredicateExecutor,
) PredicateRunner {
	return func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) error {
		res, err := executor(context.Background(), predicate, args, txo, env)
		if err != nil {
			return err
		}
		if !res {
			return errors.New(`predicate evaluated to "false"`)
		}
		return nil
	}
}
