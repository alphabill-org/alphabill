package wvm

import (
	"context"
	"fmt"

	"github.com/holiman/uint256"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const WasmSuccess = 0

type WasmVM struct {
	runtime wazero.Runtime
	mod     api.Module
}

type ExecutionCtx interface {
	GetProgramID() *uint256.Int
	GetInputData() []byte
	GetParams() []byte
	GetTxHash() []byte
}

type Storage interface {
	Read(key []byte) ([]byte, error)
	Write(key []byte, file []byte) error
}

func New(ctx context.Context, wasmSrc []byte, execCtx ExecutionCtx, opts ...Option) (*WasmVM, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.storage == nil {
		return nil, fmt.Errorf("storage is nil")
	}
	if len(wasmSrc) < 1 {
		return nil, fmt.Errorf("wasm src is missing")
	}
	rt := wazero.NewRuntimeWithConfig(ctx, options.cfg)
	if _, err := buildHostModule(ctx, rt, execCtx, options.storage); err != nil {
		return nil, fmt.Errorf("host module initialization failed, %w", err)
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

func (vm *WasmVM) CheckApiCallExists() error {
	if len(vm.mod.ExportedFunctionDefinitions()) < 1 {
		return fmt.Errorf("no exported functions")
	}
	return nil
}

func (vm *WasmVM) GetApiFn(fnName string) (api.Function, error) {
	fn := vm.mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("function %v not found", fnName)
	}
	return fn, nil
}

func ValidateResult(retVal []uint64) error {
	if len(retVal) > 1 {
		return fmt.Errorf("unexpected return value length %v", len(retVal))
	}
	// todo: translate error code to go error
	if retVal[0] != WasmSuccess {
		return fmt.Errorf("program exited with error %v", retVal[0])
	}
	return nil
}
