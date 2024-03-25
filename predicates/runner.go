package predicates

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
)

const MaxPredicateBinSize = 65536

type (
	Predicate struct {
		_      struct{} `cbor:",toarray"`
		Tag    uint64
		Code   []byte
		Params []byte
	}

	PredicateEngine interface {
		// unique ID of the engine, this is used to dispatch predicates (predicate.Tag == engine.ID)
		// to the engine which is supposed to evaluate it.
		ID() uint64
		// executes given predicate
		Execute(ctx context.Context, predicate *Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)
	}

	PredicateEngines map[uint64]func(ctx context.Context, predicate *Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)

	// environment where predicate runs (AKA transaction execution context)
	// This is meant to provide the predicate engine with access to the
	// tx system which processes the transaction.
	TxContext interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		// until AB-1012 gets resolved we need this hack to get correct payload bytes.
		PayloadBytes(txo *types.TransactionOrder) ([]byte, error)
	}
)

func (p Predicate) AsBytes() (types.PredicateBytes, error) {
	buf, err := types.Cbor.Marshal(p)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func ExtractPredicate(predicateBytes []byte) (*Predicate, error) {
	predicate := &Predicate{}
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
PredicateRunner is a helper to refactor predicate support - provide implementation which is common
for most tx systems (as of now only tokens tx system requires different PayloadBytes implementation,
see AB-1012 for details) and "translates" between two interfaces:
  - currently tx handlers do not have context.Context to pass to the predicate engine so wrapper
    returned doesn't require it and passes context.Background() to the predicate engine;
  - provide TxContext implementation for the predicate engine (currently the only functionality needed
    is to extract payload bytes from tx order);
  - currently tx systems do not differentiate between predicate evaluating to "false" vs returning
    error (ie invalid predicate or arguments) so wrapper returns error in case the predicate
    evaluates to "false".

Each instance of the wrapper is tx system / shard specific (tied to the state passed in as argument) and
thus can't be shared between "modules" which do not share the state!
*/
func PredicateRunner(
	// executor is the function which takes raw predicate binary and routes it to correct predicate engine.
	// usually it is PredicateEngines.Execute
	executor func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error),
	// state of the tx system which executes transactions using this wrapper (ie this var
	// is cached and forwarded to predicate engine on subsequent calls)
	state *state.State,
) func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error {
	env := &execEnv{state: state}
	return func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error {
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

// execEnv implements TxContext suitable for most tx systems.
type execEnv struct {
	state *state.State
}

func (ee *execEnv) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return ee.state.GetUnit(id, committed)
}

func (*execEnv) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return txo.PayloadBytes()
}
