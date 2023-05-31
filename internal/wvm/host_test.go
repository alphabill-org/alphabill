package wvm

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

const stateID = 0xaabbccdd
const StateReadError = -1
const StateWriteError = -2
const ParamsReadError = -3
const InputParamsReadError = -4

func initWvmHostTest(t *testing.T, ctx context.Context, eCtx ExecutionCtx, s Storage) *WasmVM {
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	abHostModuleFn, err := BuildABHostModule(eCtx, s)
	// init WVM
	wvm, err := New(ctx, wasm, WithHostModule(abHostModuleFn))
	require.NoError(t, err)
	return wvm
}

func Test_BuildABHostModule_StorageNil(t *testing.T) {
	execCtx := &TestExecCtx{
		id:     "empty",
		input:  make([]byte, 8),
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	abHostModuleFn, err := BuildABHostModule(execCtx, nil)
	require.ErrorContains(t, err, "storage is nil")
	require.Nil(t, abHostModuleFn)
}

func Test_BuildABHostModule_ExecutionCtxNil(t *testing.T) {
	storage := NewMemoryStorage()
	abHostModuleFn, err := BuildABHostModule(nil, storage)
	require.ErrorContains(t, err, "execution context is nil")
	require.Nil(t, abHostModuleFn)
}

func Test_buildHostAPI_SetStateFromInput(t *testing.T) {
	ctx := context.Background()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	storage := NewMemoryStorage()
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_input")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, WasmSuccess, res[0])
	// verify state was written to db
	val, err := storage.Read(util.Uint32ToBytes(stateID))
	require.NoError(t, err)
	require.True(t, bytes.Equal(val, eCtx.input))
}

func Test_buildHostAPI_SetStateFromInput_Nil(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  nil,
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_input")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, InputParamsReadError, int64(res[0]))
	// verify state was written to db
	val, err := storage.Read(util.Uint32ToBytes(stateID))
	require.Error(t, err)
	require.Nil(t, val)
}

func Test_buildHostAPI_SetStateFromInput_Empty(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{},
		params: []byte{1, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_input")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	// program expects 8 bytes
	require.EqualValues(t, InputParamsReadError, int64(res[0]))
	// verify state was written to db
	val, err := storage.Read(util.Uint32ToBytes(stateID))
	require.Error(t, err)
	require.Nil(t, val)
}

func Test_buildHostAPI_SetStateFromInput_TooLong(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0, 1},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_input")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	// program expects 8 bytes
	require.EqualValues(t, InputParamsReadError, int64(res[0]))
	// verify state was written to db
	val, err := storage.Read(util.Uint32ToBytes(stateID))
	require.Error(t, err)
	require.Nil(t, val)
}

func Test_buildHostAPI_SetStateFromParams(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_params")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, WasmSuccess, res[0])
	// verify state was written to db
	val, err := storage.Read(util.Uint32ToBytes(stateID))
	require.NoError(t, err)
	require.True(t, bytes.Equal(val, eCtx.params))
}

func Test_buildHostAPI_SetStateFromParams_TooBig(t *testing.T) {
	ctx := context.Background()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0, 1},
	}
	storage := NewMemoryStorage()
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_params")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, ParamsReadError, res[0])
}

func Test_buildHostAPI_SetStateFromParams_Nil(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: nil,
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_params")
	require.NoError(t, err)
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, ParamsReadError, res[0])
}

func Test_buildHostAPI_GetState(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	require.NoError(t, storage.Write(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0}))
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("get_state")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, 6, res[0])
}

func Test_buildHostAPI_GetState_Missing(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("get_state")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateReadError, res[0])
}

func Test_buildHostAPI_GetState_TooLong(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	require.NoError(t, storage.Write(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0, 1}))
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("get_state")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateReadError, res[0])
}

func Test_buildHostAPI_SetStateFromParamsDBWriteFails(t *testing.T) {
	ctx := context.Background()
	storage := NewAlwaysFailsMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	wvm := initWvmHostTest(t, ctx, eCtx, storage)
	fn, err := wvm.GetApiFn("set_state_params")
	require.NoError(t, err)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateWriteError, res[0])
}
