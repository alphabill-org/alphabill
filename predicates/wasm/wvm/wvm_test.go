package wvm

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/bumpallocator"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/instrument"
	"github.com/alphabill-org/alphabill/state"
)

//go:embed testdata/add_one/target/wasm32-unknown-unknown/release/add_one.wasm
var addOneWasm []byte

//go:embed testdata/conference_tickets/v2/conf_tickets.wasm
var ticketsWasm []byte

func TestNew(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)

	memDB, err := memorydb.New()
	require.NoError(t, err)

	require.NoError(t, err)
	wvm, err := New(ctx, encoder.TXSystemEncoder{}, nil, obs, WithStorage(memDB))
	require.NoError(t, err)
	require.NotNil(t, wvm)

	require.NoError(t, wvm.Close(ctx))
}

func TestReadHeapBase(t *testing.T) {
	env := &mockTxContext{
		curRound:     func() uint64 { return 1709683000 },
		GasRemaining: 100000,
	}
	enc := encoder.TXSystemEncoder{}
	conf := wasm.PredicateParams{Entrypoint: "bearer_invariant"}
	wvm, err := New(context.Background(), enc, nil, observability.Default(t))
	require.NoError(t, err)
	_, err = wvm.Exec(context.Background(), ticketsWasm, nil, conf, nil, env)
	require.Error(t, err)
	m, err := wvm.runtime.Instantiate(context.Background(), ticketsWasm)
	require.NoError(t, err)
	require.EqualValues(t, 8384, m.ExportedGlobal("__heap_base").Get())
	require.EqualValues(t, 8384, wvm.ctx.memMngr.(*bumpallocator.BumpAllocator).HeapBase())
}

//go:embed testdata/stack_height.wasm
var stackHeightWasm []byte

func Test_instrumentation_trigger(t *testing.T) {
	// if this test fails it means that either the way how instrumentation lib
	// calculates stack height or gas cost has changed or the way Wazero
	// engine works has changed. We might need to introduce new version of
	// WASM predicates to keep things deterministic?

	// These are the numbers we see for the current version of instrumentation
	// library and Wazero combo.
	type costData struct {
		arg    int32  // argument to the func
		height uint32 // minimum stack height required
		gas    uint64 // gas required
	}
	data := []costData{
		{arg: 0, height: 8, gas: 15},
		{arg: 1, height: 12, gas: 45},
		{arg: 2, height: 16, gas: 75},
		{arg: 3, height: 20, gas: 105},
		{arg: 4, height: 24, gas: 135},
		{arg: 5, height: 28, gas: 165},
	}

	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, defaultOptions().cfg)
	defer rt.Close(ctx)

	runTest := func(t *testing.T, arg int32, stackHeight uint32, gasAvailable uint64) (uint64, error) {
		instrPredicate, err := instrument.MeterGasAndStack(stackHeightWasm, stackHeight)
		if err != nil {
			t.Fatal("instrumenting predicate", err)
		}
		m, err := rt.Instantiate(ctx, instrPredicate)
		if err != nil {
			t.Fatal("failed to instantiate predicate code", err)
		}
		defer m.Close(ctx)

		gas, ok := m.ExportedGlobal(instrument.GasCounterName).(api.MutableGlobal)
		if !ok {
			t.Fatal("gas counter not found (instrumentation failed?)")
		}
		gas.Set(gasAvailable)

		fn := m.ExportedFunction("f")
		if fn == nil {
			t.Fatal("module doesn't export the 'f' function")
		}
		_, err = fn.Call(ctx, api.EncodeI32(arg))
		return gas.Get(), err
	}

	for _, tc := range data {
		t.Run(fmt.Sprintf("argument %d", tc.arg), func(t *testing.T) {
			// cfg with minimum resources which should run successfully
			gas, err := runTest(t, tc.arg, tc.height, tc.gas)
			if assert.NoError(t, err) {
				assert.Zero(t, gas, `expected remaining gas to be zero`)
			}

			// should err because of not enough stack height
			gas, err = runTest(t, tc.arg, tc.height-1, tc.gas)
			if assert.ErrorContains(t, err, `wasm error: unreachable`) {
				assert.EqualValues(t, 15, gas, `remaining gas`)
			}

			// should err because of out of gas
			gas, err = runTest(t, tc.arg, tc.height, tc.gas-1)
			if assert.ErrorContains(t, err, `wasm error: unreachable`) {
				if gas != math.MaxUint64 {
					t.Errorf("expected remaining gas to be MaxUint64, got %d", gas)
				}
			}
		})
	}
}

func Benchmark_wazero_call_wasm_fn(b *testing.B) {
	// measure overhead of calling func from WASM
	// simple wazero setup, without any AB specific stuff.
	// addOneWasm is a WASM module which exports "add_one(x: i32) -> i32"
	b.StopTimer()
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, defaultOptions().cfg)
	defer rt.Close(ctx)

	if _, err := rt.Instantiate(ctx, envWasm); err != nil {
		b.Fatalf("instantiate env module: %v", err)
	}
	m, err := rt.Instantiate(ctx, addOneWasm)
	if err != nil {
		b.Fatal("failed to instantiate predicate code", err)
	}
	defer m.Close(ctx)

	fn := m.ExportedFunction("add_one")
	if fn == nil {
		b.Fatal("module doesn't export the add_one function")
	}

	b.StartTimer()
	var n uint64
	for i := 0; i < b.N; i++ {
		res, err := fn.Call(ctx, n)
		if err != nil {
			b.Errorf("add_one returned error: %v", err)
		}
		if n = res[0]; n != uint64(i+1) {
			b.Errorf("expected %d got %d", i, n)
		}
	}
}

type mockTxContext struct {
	getUnit      func(id types.UnitID, committed bool) (*state.Unit, error)
	committedUC  func() *types.UnicityCertificate
	curRound     func() uint64
	trustBase    func() (types.RootTrustBase, error)
	GasRemaining uint64
	calcCost     func() uint64
	exArgument   func() ([]byte, error)
}

func (env *mockTxContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return env.getUnit(id, committed)
}

func (env *mockTxContext) CommittedUC() *types.UnicityCertificate { return env.committedUC() }

func (env *mockTxContext) CurrentRound() uint64 { return env.curRound() }

func (env *mockTxContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return env.trustBase()
}

func (env *mockTxContext) GasAvailable() uint64 {
	return env.GasRemaining
}

func (env *mockTxContext) SpendGas(gas uint64) error {
	if gas > env.GasRemaining {
		env.GasRemaining = 0
		return fmt.Errorf("out of gas")
	}
	env.GasRemaining -= gas
	return nil
}

func (env *mockTxContext) CalculateCost() uint64 { return env.calcCost() }

func (env *mockTxContext) ExtraArgument() ([]byte, error) { return env.exArgument() }

type mockRootTrustBase struct {
	verifyQuorumSignatures func(data []byte, signatures map[string]hex.Bytes) (error, []error)

	// instead of implementing all methods just embed the interface for now
	types.RootTrustBase
}

func (rtb *mockRootTrustBase) VerifyQuorumSignatures(data []byte, signatures map[string]hex.Bytes) (error, []error) {
	return rtb.verifyQuorumSignatures(data, signatures)
}
