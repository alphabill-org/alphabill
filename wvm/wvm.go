package wvm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type WasmVM struct {
	runtime wazero.Runtime
	mod     api.Module
}

// New - creates new wazero based wasm vm
func New(ctx context.Context, wasmSrc []byte, opts ...Option) (*WasmVM, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if len(wasmSrc) < 1 {
		return nil, fmt.Errorf("wasm src is missing")
	}
	rt := wazero.NewRuntimeWithConfig(ctx, options.cfg)
	for _, m := range options.hostMod {
		if _, err := m(ctx, rt); err != nil {
			return nil, fmt.Errorf("host module initialization failed, %w", err)
		}
	}
	m, err := rt.Instantiate(ctx, wasmSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate VM with wasm source, %w", err)
	}
	return &WasmVM{
		runtime: rt,
		mod:     m,
	}, nil
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
