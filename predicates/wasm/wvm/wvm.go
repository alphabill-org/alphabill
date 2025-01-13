package wvm

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/bumpallocator"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/instrument"
	"github.com/alphabill-org/alphabill/state"
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
	handle_current_tx_order = 1 // tx order which triggered the predicate
	handle_current_args     = 2 // user supplied arguments to the tx
	handle_predicate_conf   = 3 // BLOB saved with the predicate binary
	handle_max_reserved     = 10
)

type (
	allocator interface {
		Alloc(mem bumpallocator.Memory, size uint32) (uint32, error)
		Free(mem bumpallocator.Memory, ptr uint32) error
	}

	vmContext struct {
		memMngr allocator
		storage keyvaluedb.KeyValueDB
		curPrg  *evalContext
		encoder Encoder
		factory ABTypesFactory
		engines predicates.PredicateExecutor
		log     *slog.Logger
	}

	WasmVM struct {
		runtime wazero.Runtime
		ctx     *vmContext
	}

	// "evaluation context" of current program
	evalContext struct {
		mod    api.Module // created from the WASM of the predicate
		vars   map[uint64]any
		varIdx uint64          // "handle generator" for vars
		env    EvalEnvironment // callback to the tx system
	}

	EvalEnvironment interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		CurrentRound() uint64
		TrustBase(epoch uint64) (types.RootTrustBase, error)
		GasAvailable() uint64
		SpendGas(gas uint64) error
		CalculateCost() uint64
		TransactionOrder() (*types.TransactionOrder, error)
	}

	// translates AB types to WASM consumable representation
	Encoder interface {
		Encode(obj any, ver uint32, getHandle func(obj any) uint64) ([]byte, error)
		TxAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error)
		UnitData(unit *state.Unit, ver uint32) ([]byte, error)
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
	}
)

/*
addVar adds the "obj" into list of variables in current context and returns it's handle
*/
func (ec *evalContext) addVar(obj any) uint64 {
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

/*
getBytesVariable returns "[]byte compatible" variable as []byte (the getVar
generic implementation can only return exact type, not underlying type)
*/
func (vmc *vmContext) getBytesVariable(handle uint64) ([]byte, error) {
	v, ok := vmc.curPrg.vars[handle]
	if !ok {
		return nil, fmt.Errorf("variable with handle %d not found", handle)
	}

	switch d := v.(type) {
	case []byte:
		return d, nil
	case types.RawCBOR:
		return d, nil
	default:
		return nil, fmt.Errorf("can't handle var of type %T", v)
	}
}

/*
reset clears the "variable part" (specific to the predicate evaluated) of
the context.
*/
func (vmc *vmContext) reset() {
	vmc.curPrg.mod = nil
	vmc.curPrg.env = nil
	clear(vmc.curPrg.vars)
}

func (vmCtx *vmContext) writeToMemory(mod api.Module, buf []byte) (uint64, error) {
	if mod == nil {
		return 0, errors.New("module is unassigned")
	}
	mem := mod.Memory()
	if mem == nil {
		return 0, errors.New("module doesn't export memory")
	}

	size := uint32(len(buf))
	addr, err := vmCtx.memMngr.Alloc(mem, size)
	if err != nil {
		return 0, fmt.Errorf("allocating memory: %w", err)

	}
	if ok := mem.Write(addr, buf); !ok {
		return 0, errors.New("out of range when writing data into memory")
	}

	return api.EncodeI64(int64(newPointerSize(addr, size))), nil
}

// New - creates new wazero based wasm vm
func New(ctx context.Context, enc Encoder, engines predicates.PredicateExecutor, observe Observability, opts ...Option) (*WasmVM, error) {
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
		return nil, fmt.Errorf("adding host module: %w", err)
	}
	// predicate execution context API
	if err := addContextModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("adding current eval context module: %w", err)
	}
	if err := addCBORModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("adding CBOR API module: %w", err)
	}
	if err := addAlphabillModule(ctx, rt, observe); err != nil {
		return nil, fmt.Errorf("adding alphabill API module: %w", err)
	}

	return &WasmVM{
		runtime: rt,
		ctx: &vmContext{
			curPrg: &evalContext{
				vars: map[uint64]any{},
			},
			encoder: enc,
			engines: engines,
			factory: ABTypesFactory{},
			storage: options.storage,
			log:     observe.Logger(),
		},
	}, nil
}

/*
Exec loads the WASM module passed in as "predicate" argument and calls the "conf.Entrypoint" function in it.
  - The entrypoint function signature must be "no parameters and single i64 return value" where
    zero means "true" and non-zero is "false" (ie the returned number is error code);
*/
func (vm *WasmVM) Exec(ctx context.Context, predicate, args []byte, conf wasm.PredicateParams, sigBytesFn func() ([]byte, error), env EvalEnvironment) (uint64, error) {
	if len(predicate) < 1 {
		return 0, fmt.Errorf("predicate is nil")
	}
	// Generally we expect long running / buggy predicates to be terminated because
	// of running out of gas; we do set the stack height limit as last resort
	// safety measure / to keep things deterministic...
	// Hardcoded to 64K for now, judged to be enough until requirements are refined.
	instrPredicate, err := instrument.MeterGasAndStack(predicate, 1<<16)
	if err != nil {
		return 0, fmt.Errorf("instrumenting predicate error: %w", err)
	}
	m, err := vm.runtime.Instantiate(ctx, instrPredicate)
	if err != nil {
		return 0, fmt.Errorf("failed to instantiate predicate code: %w", err)
	}
	defer m.Close(ctx)

	heapBase := m.ExportedGlobal("__heap_base")
	if heapBase == nil {
		return 0, fmt.Errorf("__heap_base is not exported from the predicate module")
	}
	gas, ok := m.ExportedGlobal(instrument.GasCounterName).(api.MutableGlobal)
	if !ok {
		return 0, fmt.Errorf("instrumentation failed, gas counter not found")
	}
	initialGas := env.GasAvailable()
	gas.Set(initialGas)

	defer vm.ctx.reset()
	vm.ctx.memMngr = bumpallocator.New(api.DecodeU32(heapBase.Get()), m.Memory().Definition())
	vm.ctx.curPrg.mod = m
	vm.ctx.curPrg.env = env
	vm.ctx.curPrg.varIdx = handle_max_reserved
	//vm.ctx.curPrg.vars[handle_current_tx_order] = txo // TODO AB-1724
	vm.ctx.curPrg.vars[handle_current_args] = args
	vm.ctx.curPrg.vars[handle_predicate_conf] = conf.Args

	fn := m.ExportedFunction(conf.Entrypoint)
	if fn == nil {
		return 0, fmt.Errorf("module doesn't export function %q", conf.Entrypoint)
	}

	res, evalErr := fn.Call(context.WithValue(ctx, runtimeContextKey, vm.ctx))
	gasRemaining := gas.Get()
	// out of gas during wasm predicate execution, in case of out of gas the Call will
	// also return unreachable error, but we will instead return gas calculation error
	if evalErr != nil && (gasRemaining == math.MaxUint64 || gasRemaining > initialGas) {
		// force error, spend whole budget
		return 0, errors.Join(evalErr, env.SpendGas(math.MaxUint64))
	}
	// spend gas according to how much was used. the execution of the predicate might have
	// failed but we take the fee (gas) anyway!
	if err := env.SpendGas(initialGas - gasRemaining); err != nil {
		return 0, errors.Join(evalErr, fmt.Errorf("calculating gas usage: %w", err))
	}
	if evalErr != nil {
		return 0, fmt.Errorf("calling %s returned error: %w", conf.Entrypoint, evalErr)
	}

	vm.ctx.log.DebugContext(ctx, fmt.Sprintf("%s.%s.RESULT: %#v", m.Name(), conf.Entrypoint, res))
	if len(res) != 1 {
		return 0, fmt.Errorf("unexpected return value length %v", len(res))
	}
	return res[0], nil
}

func (vm *WasmVM) Close(ctx context.Context) error {
	return vm.runtime.Close(ctx)
}

func extractVMContext(ctx context.Context) *vmContext {
	rtCtx := ctx.Value(runtimeContextKey).(*vmContext)
	if rtCtx == nil {
		// when ctx doesn't contain the value something has gone very wrong...
		panic("context doesn't contain VM context value")
	}
	return rtCtx
}

/*
hostAPI allows to use more convenient function signature for implementing Wazero host
module functions.
  - the execution context is extracted from env and passed as param to the func;
  - when API func returns error we stop the execution of the predicate;
*/
func hostAPI(f func(vec *vmContext, mod api.Module, stack []uint64) error) api.GoModuleFunc {
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		rtCtx := ctx.Value(runtimeContextKey).(*vmContext)
		if rtCtx == nil {
			// when ctx doesn't contain the value something has gone very wrong...
			// instead of panic attempt to close the module?
			panic("context doesn't contain VM context value")
		}
		if err := f(rtCtx, mod, stack); err != nil {
			rtCtx.log.ErrorContext(ctx, "host API returned error", logger.Error(err))
			if err = rtCtx.curPrg.mod.CloseWithExitCode(ctx, 0xBAD00BAD); err != nil {
				rtCtx.log.ErrorContext(ctx, "host API close with exit", logger.Error(err))
			}
		}
	}
}

type EvalResult int

const (
	EvalResultTrue  EvalResult = 1
	EvalResultFalse EvalResult = 2
	EvalResultError EvalResult = 3
)

func PredicateEvalResult(code uint64) (EvalResult, uint64) {
	switch {
	case code == 0:
		return EvalResultTrue, 0
	case code&0xFF == 1:
		return EvalResultFalse, code >> 8
	default:
		return EvalResultError, code
	}
}
