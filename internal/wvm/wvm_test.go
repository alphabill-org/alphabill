package wvm

import (
	"context"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/counter.wasm
var counterWasm []byte

//go:embed testdata/empty.wasm
var emptyWasm []byte

//go:embed testdata/add_one.wasm
var addWasm []byte

type TestExecCtx struct {
	id     string
	input  []byte
	params []byte
	txHash []byte
}

func (t TestExecCtx) GetProgramID() *uint256.Int {
	return uint256.NewInt(0).SetBytes([]byte(t.id))
}

func (t TestExecCtx) GetInputData() []byte {
	return t.input
}

func (t TestExecCtx) GetParams() []byte {
	return t.params
}

func (t TestExecCtx) GetTxHash() []byte {
	return t.txHash
}

func TestNewNoHostModules(t *testing.T) {
	ctx := context.Background()
	wvm, err := New(ctx, addWasm)
	require.NoError(t, err)
	require.NotNil(t, wvm)
	fn, err := wvm.GetApiFn("add_one")
	require.NoError(t, err)
	require.NotNil(t, fn)
	res, err := fn.Call(ctx, uint64(3))
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, 4, res[0])
}

func TestNew_RequiresABModuleAndFails(t *testing.T) {
	ctx := context.Background()
	wvm, err := New(ctx, counterWasm)
	require.ErrorContains(t, err, "failed to initiate VM with wasm source, module[ab] not instantiated")
	require.Nil(t, wvm)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)
	execCtx := &TestExecCtx{
		id:     "counter",
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	abHostModuleFn, err := BuildABHostModule(execCtx, storage)
	require.NoError(t, err)
	wvm, err := New(ctx, counterWasm, WithHostModule(abHostModuleFn))
	require.NoError(t, err)
	require.NotNil(t, wvm)
}

func TestWasmVM_CheckApiCallExists_OK(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)

	execCtx := &TestExecCtx{
		id:     "counter",
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	abHostModuleFn, err := BuildABHostModule(execCtx, storage)
	require.NoError(t, err)
	wvm, err := New(ctx, counterWasm, WithHostModule(abHostModuleFn))
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
}

// loads src that does not export any functions
func TestWasmVM_CheckApiCallExists_NOK(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)

	execCtx := &TestExecCtx{
		id:     "empty",
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	abHostModuleFn, err := BuildABHostModule(execCtx, storage)
	require.NoError(t, err)
	wvm, err := New(ctx, emptyWasm, WithHostModule(abHostModuleFn))
	require.NoError(t, err)
	require.ErrorContains(t, wvm.CheckApiCallExists(), "no exported functions")
}

func TestWasmVM_GetApiFn(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)

	execCtx := &TestExecCtx{
		id:     "counter",
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	abHostModuleFn, err := BuildABHostModule(execCtx, storage)
	require.NoError(t, err)
	wvm, err := New(ctx, counterWasm, WithHostModule(abHostModuleFn))
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
	fn, err := wvm.GetApiFn("count")
	require.NoError(t, err)
	require.NotNil(t, fn)
	fn, err = wvm.GetApiFn("help")
	require.Error(t, err)
	require.Nil(t, fn)
}
