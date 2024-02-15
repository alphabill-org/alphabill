package wvm

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/allocator"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// envWasm was compiled using `wat2wasm --debug-names env.wat`
//
//go:embed ab_env.wasm
var envWasm []byte

type rtCtxKey string

const runtimeContextKey = rtCtxKey("rt.Ctx")

type (
	Allocator interface {
		Allocate(mem allocator.Memory, size uint32) (uint32, error)
		Deallocate(mem allocator.Memory, ptr uint32) error
	}

	VmContext struct {
		Alloc   Allocator
		Storage keyvaluedb.KeyValueDB
		AbCtx   *AbContext
		Log     *slog.Logger
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
		WithFunc(storageReadV1).Export("storage_read").
		NewFunctionBuilder().WithFunc(storageWriteV1).Export("storage_write").
		NewFunctionBuilder().WithFunc(p2pkhV1).Export("p2pkh_v1").
		NewFunctionBuilder().WithFunc(p2pkhV2).Export("p2pkh_v2").
		NewFunctionBuilder().WithFunc(loggingV1).Export("log_v1").
		NewFunctionBuilder().WithFunc(extFree).Export("ext_free").
		NewFunctionBuilder().WithFunc(extMalloc).Export("ext_malloc").
		Instantiate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init host module: %w", err)
	}
	_, err = rt.Instantiate(ctx, envWasm)
	if err != nil {
		return nil, errors.Join(err, rt.Close(ctx))
	}
	m, err := rt.Instantiate(ctx, wasmSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate VM with wasm source, %w", err)
	}

	global := m.ExportedGlobal("__heap_base")
	if global == nil {
		return nil, fmt.Errorf("wazero error: nil global for __heap_base")
	}

	hb := api.DecodeU32(global.Get())
	// hb = runtime.DefaultHeapBase

	mem := m.Memory()
	if mem == nil {
		return nil, fmt.Errorf("wazero error: nil memory for module")
	}

	allocator := allocator.NewFreeingBumpHeapAllocator(hb)

	return &WasmVM{
		runtime: rt,
		mod:     m,
		ctx: &VmContext{
			Alloc:   allocator,
			Storage: options.storage,
			Log:     log,
			AbCtx:   abCtx,
		},
	}, nil
}

func (vm *WasmVM) Exec(ctx context.Context, fName string, args ...[]byte) (_ bool, err error) {
	// check API
	fn, err := vm.getApiFn(fName)
	if err != nil {
		return false, fmt.Errorf("find program entry point (%s): %w", fName, err)
	}

	// copy inputs to linear memory
	mem := vm.runtime.Module("env").Memory()
	if mem == nil {
		return false, fmt.Errorf("failed to access memory exported by env module: %w", err)
	}
	var argPtrs = make([]uint64, len(args))
	for i, arg := range args {
		dataLength := uint32(len(arg))
		inputPtr, err := vm.ctx.Alloc.Allocate(mem, dataLength)
		if err != nil {
			return false, fmt.Errorf("allocating input memory failed: %w", err)
		}
		// Store the data into memory
		if ok := mem.Write(inputPtr, arg); !ok {
			return false, fmt.Errorf("failed write argument to wasm memory: %w", err)
		}
		argPtrs[i] = newPointerSize(inputPtr, dataLength)
	}
	// API calls have no parameters, there is a host callback to get input parameters
	// all programs must complete in 100 ms, this will later be replaced with gas cost
	// for now just set a hard limit to make sure programs do not run forever
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	res, err := fn.Call(context.WithValue(ctx, runtimeContextKey, vm.ctx), argPtrs...)
	if err != nil {
		return false, fmt.Errorf("calling %s returned error: %w", fName, err)
	}
	if len(res) != 1 {
		return false, fmt.Errorf("unexpected return value length %v", len(res))
	}
	return res[0] > 0, nil
}

// CheckApiCallExists check if wasm module exports any function calls
func (vm *WasmVM) CheckApiCallExists() error {
	if len(vm.mod.ExportedFunctionDefinitions()) < 1 {
		return fmt.Errorf("no exported functions")
	}
	return nil
}

// getApiFn find function "fnName" and return it
func (vm *WasmVM) getApiFn(fnName string) (api.Function, error) {
	fn := vm.mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("function %v not found", fnName)
	}
	return fn, nil
}

func (vm *WasmVM) Close(ctx context.Context) error {
	return vm.runtime.Close(ctx)
}
