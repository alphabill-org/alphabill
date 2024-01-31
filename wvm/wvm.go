package wvm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/types"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type rtCtxKey string

const runtimeContextKey = rtCtxKey("rt.Ctx")

type (
	VmContext struct {
		InputArgs []byte
		Storage   keyvaluedb.KeyValueDB
		AbCtx     *AbContext
		Log       *slog.Logger
	}

	WasmVM struct {
		runtime wazero.Runtime
		mod     api.Module
		ctx     *VmContext
	}

	AbContext struct {
		SysID    types.SystemID
		Round    uint64
		Txo      *types.TransactionOrder
		InitArgs []byte
	}
)

// New - creates new wazero based wasm vm
func New(ctx context.Context, wasmSrc []byte, abCtx *AbContext, log *slog.Logger, opts ...Option) (*WasmVM, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if len(wasmSrc) < 1 {
		return nil, fmt.Errorf("wasm src is missing")
	}
	rt := wazero.NewRuntimeWithConfig(ctx, options.cfg)
	_, err := rt.NewHostModuleBuilder("ab").
		NewFunctionBuilder().
		WithFunc(getStateV1).Export("get_state").
		NewFunctionBuilder().WithFunc(setStateV1).Export("set_state").
		NewFunctionBuilder().WithFunc(getInitValueV1).Export("get_params").
		NewFunctionBuilder().WithFunc(getInputData).Export("get_input_params").
		NewFunctionBuilder().WithFunc(p2pkhV1).Export("p2pkh_v1").
		Instantiate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init host module: %w", err)
	}
	m, err := rt.Instantiate(ctx, wasmSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate VM with wasm source, %w", err)
	}
	return &WasmVM{
		runtime: rt,
		mod:     m,
		ctx: &VmContext{
			Storage: options.storage,
			Log:     log,
			AbCtx:   abCtx,
		},
	}, nil
}

func (vm *WasmVM) Exec(ctx context.Context, fName string, data []byte) (_ bool, err error) {
	// check API
	fn, err := vm.GetApiFn(fName)
	if err != nil {
		return false, fmt.Errorf("program call failed, %w", err)
	}
	// set input data
	vm.ctx.InputArgs = data
	runCtx := context.WithValue(context.Background(), runtimeContextKey, vm.ctx)
	// API calls have no parameters, there is a host callback to get input parameters
	// all programs must complete in 100 ms, this will later be replaced with gas cost
	// for now just set a hard limit to make sure programs do not run forever
	ctx, cancel := context.WithTimeout(runCtx, 100*time.Millisecond)
	defer cancel()
	res, err := fn.Call(ctx)
	if err != nil {
		return false, fmt.Errorf("program call failed, %w", err)
	}
	if len(res) > 1 {
		return false, fmt.Errorf("unexpected return value length %v", len(res))
	}
	return res[0] >= 0, nil
}

// CheckApiCallExists check if wasm module exports any function calls
func (vm *WasmVM) CheckApiCallExists() error {
	if len(vm.mod.ExportedFunctionDefinitions()) < 1 {
		return fmt.Errorf("no exported functions")
	}
	return nil
}

// GetApiFn find function "fnName" and return it
func (vm *WasmVM) GetApiFn(fnName string) (api.Function, error) {
	fn := vm.mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("function %v not found", fnName)
	}
	return fn, nil
}

func (vm *WasmVM) Close(ctx context.Context) error {
	return vm.runtime.Close(ctx)
}
