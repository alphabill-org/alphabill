package wasm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
	exec "github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm"
)

type WasmRunner struct {
	vm      *wvm.WasmVM
	log     *slog.Logger
	execDur metric.Float64Histogram
}

func New(enc wvm.Encoder, engines exec.PredicateExecutor, orchestration wvm.Orchestration, obs wvm.Observability) (WasmRunner, error) {
	vm, err := wvm.New(context.Background(), enc, engines, orchestration, obs)
	if err != nil {
		return WasmRunner{}, fmt.Errorf("creating WASM engine: %w", err)
	}

	m := obs.Meter("predicates.wasm")
	execDur, err := m.Float64Histogram("exec.time",
		metric.WithDescription("How long it took to execute an predicate"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.13))
	if err != nil {
		return WasmRunner{}, fmt.Errorf("creating histogram for predicate execution time: %w", err)
	}

	return WasmRunner{vm: vm, log: obs.Logger(), execDur: execDur}, nil
}

func (WasmRunner) ID() uint64 {
	return wasm.PredicateEngineID
}

func (wr WasmRunner) Execute(ctx context.Context, p *predicates.Predicate, args []byte, txo *types.TransactionOrder, env exec.TxContext) (bool, error) {
	if p.Tag != wasm.PredicateEngineID {
		return false, fmt.Errorf("expected predicate engine tag %d but got %d", wasm.PredicateEngineID, p.Tag)
	}
	defer func(start time.Time) { wr.execDur.Record(ctx, time.Since(start).Seconds()) }(time.Now())

	par := wasm.PredicateParams{}
	if err := types.Cbor.Unmarshal(p.Params, &par); err != nil {
		return false, fmt.Errorf("decoding predicate parameters: %w", err)
	}

	code, err := wr.vm.Exec(ctx, p.Code, args, par, txo, env)
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
