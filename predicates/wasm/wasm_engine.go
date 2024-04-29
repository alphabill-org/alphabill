package wasm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm"
	"github.com/alphabill-org/alphabill/types"
)

const wasmEngineID = 1

type WasmRunner struct {
	vm  *wvm.WasmVM
	log *slog.Logger
}

func New(enc wvm.Encoder, obs wvm.Observability) WasmRunner {
	vm, err := wvm.New(context.Background(), enc, obs)
	if err != nil {
		panic(fmt.Errorf("creating WASM engine: %w", err))
	}
	return WasmRunner{vm: vm, log: obs.Logger()}
}

func (WasmRunner) ID() uint64 {
	return wasmEngineID
}

func (wr WasmRunner) Execute(ctx context.Context, p *predicates.Predicate, args []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
	if p.Tag != wasmEngineID {
		return false, fmt.Errorf("expected predicate engine tag %d but got %d", wasmEngineID, p.Tag)
	}

	par := WasmPredicateParams{}
	if err := types.Cbor.Unmarshal(p.Params, &par); err != nil {
		return false, fmt.Errorf("decoding predicate parameters: %w", err)
	}

	code, err := wr.vm.Exec(ctx, par.Entripoint, p.Code, args, txo, env)
	if err != nil {
		return false, fmt.Errorf("executing predicate: %w", err)
	}
	er, code := wvm.PredicateEvalResult(code)
	switch er {
	case wvm.EvalResultTrue:
		return true, nil
	case wvm.EvalResultFalse:
		wr.log.DebugContext(ctx, fmt.Sprintf("%s predicate evaluated to 'false' with code %x", par.Entripoint, code))
		return false, nil
	case wvm.EvalResultError:
		return false, fmt.Errorf("predicate returned error code %x", code)
	}

	return false, fmt.Errorf("WASM engine not implemented; exec %#v & %v", p, args)
}

/*
WasmPredicateParams is the data struct passed in in Predicate.Params field.
*/
type WasmPredicateParams struct {
	_          struct{} `cbor:",toarray"`
	Entripoint string   // func name to call from the WASM binary
}
