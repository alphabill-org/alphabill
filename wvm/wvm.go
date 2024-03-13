package wvm

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/allocator"
)

// WASM of "env" module which exports memory so data can be shared between host
// and other WASM module(s).
// envWasm was compiled using `wat2wasm --debug-names env.wat`
//
//go:embed ab_env.wasm
var envWasm []byte

type rtCtxKey string

const runtimeContextKey = rtCtxKey("rt.Ctx")

const (
	handle_current_tx_order = 1
	handle_current_args     = 2
	handle_max_reserved     = 10
)

type (
	Allocator interface {
		Allocate(mem allocator.Memory, size uint32) (uint32, error)
		Deallocate(mem allocator.Memory, ptr uint32) error
	}

	VmContext struct {
		Alloc   Allocator
		Storage keyvaluedb.KeyValueDB
		curPrg  *EvalContext
		encoder Encoder
		factory ABTypesFactory
		log     *slog.Logger
	}

	WasmVM struct {
		runtime wazero.Runtime
		ctx     *VmContext
	}

	// "evaluation context" of current program
	EvalContext struct {
		mod    api.Module // created from the WASM of the predicate
		args   []byte     // predicate arguments
		vars   map[uint64]any
		varIdx uint64          // "handle generator" for vars
		env    EvalEnvironment // callback to the tx system
	}

	EvalEnvironment interface {
		//Factory(typeID uint32, data []byte) (any, error)
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		PayloadBytes(txo *types.TransactionOrder) ([]byte, error)
		CurrentRound() uint64
		TrustBase() (map[string]abcrypto.Verifier, error)
	}

	// translates AB types to WASM consumable representation
	Encoder interface {
		Encode(obj any, getHandler handlerFunc) ([]byte, error)
		TxAttributes(txo *types.TransactionOrder) ([]byte, error)
		UnitData(unit *state.Unit) ([]byte, error)
	}

	Observability interface {
		//Tracer(name string, options ...trace.TracerOption) trace.Tracer
		//Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
	}
)

/*
AddVar adds the "obj" into list of variables in current context and returns it's handle
*/
func (ec *EvalContext) AddVar(obj any) uint64 {
	ec.varIdx++
	ec.vars[ec.varIdx] = obj
	return ec.varIdx
}

func getVar[T any](vars map[uint64]any, handle uint64) (T, error) {
	var e T
	v, ok := vars[handle]
	if !ok {
		return e, fmt.Errorf("invalid handle %d (not found)", handle)
	}
	e, ok = v.(T)
	if !ok {
		return e, fmt.Errorf("handle %d is for %T, not %T", handle, v, e)
	}
	return e, nil
}

func (vmc *VmContext) EndEval() {
	vmc.curPrg.args = nil
	vmc.curPrg.mod = nil
	clear(vmc.curPrg.vars)
}

func (vmCtx *VmContext) writeToMemory(mod api.Module, buf []byte) (uint64, error) {
	if mod == nil {
		return 0, errors.New("module is unassigned")
	}
	mem := mod.Memory()
	if mem == nil {
		return 0, errors.New("module doesn't export memory")
	}

	size := uint32(len(buf))
	addr, err := vmCtx.Alloc.Allocate(mem, size)
	if err != nil {
		return 0, fmt.Errorf("allocating memory: %w", err)

	}
	if ok := mem.Write(addr, buf); !ok {
		return 0, errors.New("out of range when writing data into memory")
	}

	return api.EncodeI64(int64(newPointerSize(addr, size))), nil
}

// New - creates new wazero based wasm vm
func New(ctx context.Context, env EvalEnvironment, observe Observability, opts ...Option) (*WasmVM, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	rt := wazero.NewRuntimeWithConfig(ctx, options.cfg)
	// WASM shared memory env
	if _, err := rt.Instantiate(ctx, envWasm); err != nil {
		return nil, errors.Join(fmt.Errorf("instantiate env module: %w", err), rt.Close(ctx))
	}
	// host utility APIs
	if err := addHostModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("failed to init host module: %w", err)
	}
	// predicate execution context API
	if err := addContextModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("adding current eval context module: %w", err)
	}

	if err := addAlphabillModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("adding alphabill API module: %w", err)
	}

	return &WasmVM{
		runtime: rt,
		ctx: &VmContext{
			curPrg: &EvalContext{
				env:  env,
				vars: map[uint64]any{},
			},
			encoder: TXSystemEncoder{},
			factory: ABTypesFactory{},
			Storage: options.storage,
			log:     observe.Logger(),
		},
	}, nil
}

/*
Exec loads the WASM module in "predicate" and calls the "fName" function in it.
  - "fName" function signature must be no parameters and single i64 return value where
    zero means "true" and non-zero is "false" (ie the returned number is error code);
*/
func (vm *WasmVM) Exec(ctx context.Context, fName string, predicate, args []byte, txo *types.TransactionOrder) (_ uint64, err error) {
	if len(predicate) < 1 {
		return 0, fmt.Errorf("predicate is nil")
	}
	m, err := vm.runtime.Instantiate(ctx, predicate)
	if err != nil {
		return 0, fmt.Errorf("failed to instantiate predicate code: %w", err)
	}
	defer m.Close(ctx)

	global := m.ExportedGlobal("__heap_base")
	if global == nil {
		return 0, fmt.Errorf("__heap_base is not exported from the predicate module")
	}

	fn := m.ExportedFunction("_ab_sdk_version")
	if fn != nil {
		rsp, err := fn.Call(ctx)
		vm.ctx.log.DebugContext(ctx, fmt.Sprintf("SDK: %v = %v", rsp, err))
	}

	// do we need to create new mem manager for each predicate?
	hb := api.DecodeU32(global.Get())
	vm.ctx.Alloc = allocator.NewFreeingBumpHeapAllocator(hb)
	vm.ctx.curPrg.mod = m
	vm.ctx.curPrg.args = args
	vm.ctx.curPrg.varIdx = handle_max_reserved
	defer vm.ctx.EndEval()
	vm.ctx.curPrg.vars[handle_current_tx_order] = txo
	vm.ctx.curPrg.vars[handle_current_args] = args

	fn = m.ExportedFunction(fName)
	if fn == nil {
		return 0, fmt.Errorf("module doesn't export function %q", fName)
	}

	// all programs must complete in 100 ms, this will later be replaced with gas cost
	// for now just set a hard limit to make sure programs do not run forever
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	res, err := fn.Call(context.WithValue(ctx, runtimeContextKey, vm.ctx))
	if err != nil {
		return 0, fmt.Errorf("calling %s returned error: %w", fName, err)
	}
	if len(res) != 1 {
		return 0, fmt.Errorf("unexpected return value length %v", len(res))
	}
	vm.ctx.log.DebugContext(ctx, fmt.Sprintf("%s.%s.RESULT: %#v", m.Name(), fName, res))
	return res[0], nil
}

func (vm *WasmVM) Close(ctx context.Context) error {
	return vm.runtime.Close(ctx)
}

func vmContext(ctx context.Context) *VmContext {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		// when ctx doesn't contain the value something has gone very wrong...
		panic("context doesn't contain VM context value")
	}
	return rtCtx
}

func hostAPI(f func(vec *VmContext, mod api.Module, stack []uint64) error) api.GoModuleFunc {
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
		if rtCtx == nil {
			// when ctx doesn't contain the value something has gone very wrong...
			panic("context doesn't contain VM context value")
		}
		if err := f(rtCtx, mod, stack); err != nil {
			rtCtx.log.Error("host API returned error", logger.Error(err))
			rtCtx.curPrg.mod.CloseWithExitCode(ctx, 0xFF0000FF)
		}
	}
}

type mockTxContext struct {
	getUnit      func(id types.UnitID, committed bool) (*state.Unit, error)
	payloadBytes func(txo *types.TransactionOrder) ([]byte, error)
	curRound     func() uint64
	trustBase    func() (map[string]abcrypto.Verifier, error)
}

func (env *mockTxContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return env.getUnit(id, committed)
}
func (env *mockTxContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return env.payloadBytes(txo)
}

func (env *mockTxContext) CurrentRound() uint64                             { return env.curRound() }
func (env *mockTxContext) TrustBase() (map[string]abcrypto.Verifier, error) { return env.trustBase() }
