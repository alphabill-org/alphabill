package wasm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
	exec "github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm"
)

type WasmRunner struct {
	vm  *wvm.WasmVM
	log *slog.Logger
}

func New(enc wvm.Encoder, engines exec.PredicateExecutor, obs wvm.Observability) WasmRunner {
	vm, err := wvm.New(context.Background(), enc, engines, obs)
	if err != nil {
		panic(fmt.Errorf("creating WASM engine: %w", err))
	}
	return WasmRunner{vm: vm, log: obs.Logger()}
}

func (WasmRunner) ID() uint64 {
	return wasm.PredicateEngineID
}

func (wr WasmRunner) Execute(ctx context.Context, p *predicates.Predicate, args []byte, sigBytesFn func() ([]byte, error), env exec.TxContext) (bool, error) {
	if p.Tag != wasm.PredicateEngineID {
		return false, fmt.Errorf("expected predicate engine tag %d but got %d", wasm.PredicateEngineID, p.Tag)
	}

	par := wasm.PredicateParams{}
	if err := types.Cbor.Unmarshal(p.Params, &par); err != nil {
		return false, fmt.Errorf("decoding predicate parameters: %w", err)
	}

	code, err := wr.vm.Exec(ctx, p.Code, args, par, sigBytesFn, env)
	if err != nil {
		return false, fmt.Errorf("executing predicate: %w", err)
	}
	er, code := wvm.PredicateEvalResult(code)
	switch er {
	case wvm.EvalResultTrue:
		return true, nil
	case wvm.EvalResultFalse:
		wr.log.DebugContext(ctx, fmt.Sprintf("%s predicate evaluated to 'false' with code %x", par.Entrypoint, code))
		return false, nil
	case wvm.EvalResultError:
		return false, fmt.Errorf("predicate returned error code %x", code)
	default:
		return false, fmt.Errorf("unexpected evaluation result %v (%x)", er, code)
	}
}
