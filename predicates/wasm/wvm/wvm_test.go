package wvm

import (
	"context"
	_ "embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"

	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/allocator"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
)

//go:embed testdata/add_one/target/wasm32-unknown-unknown/release/add_one.wasm
var addOneWasm []byte

//go:embed testdata/missingHostAPI/target/wasm32-unknown-unknown/release/invalid_api.wasm
var invalidAPIWasm []byte

//go:embed testdata/p2pkh_v1/p2pkh.wasm
var p2pkhV1Wasm []byte

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
	require.EqualValues(t, 8400, m.ExportedGlobal("__heap_base").Get())
	require.EqualValues(t, 8400, wvm.ctx.MemMngr.(*allocator.BumpAllocator).HeapBase())
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
	payloadBytes func(txo *types.TransactionOrder) ([]byte, error)
	curRound     func() uint64
	trustBase    func() (types.RootTrustBase, error)
	GasRemaining uint64
	calcCost     func() uint64
}

func (env *mockTxContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return env.getUnit(id, committed)
}

func (env *mockTxContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return env.payloadBytes(txo)
}

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

type mockRootTrustBase struct {
	verifyQuorumSignatures func(data []byte, signatures map[string][]byte) (error, []error)

	// instead of implementing all methods just embed the interface for now
	types.RootTrustBase
}

func (rtb *mockRootTrustBase) VerifyQuorumSignatures(data []byte, signatures map[string][]byte) (error, []error) {
	return rtb.verifyQuorumSignatures(data, signatures)
}
