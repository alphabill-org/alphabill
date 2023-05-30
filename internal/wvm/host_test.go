package wvm

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

const stateID = 0xaabbccdd
const StateReadError = -1
const StateWriteError = -2
const ParamsReadError = -3
const InputParamsReadError = -4

func Test_buildHostAPI_SetStateFromInput(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_input")
	require.NotNil(t, fn)
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
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  nil,
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_input")
	require.NotNil(t, fn)
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
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{},
		params: []byte{1, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_input")
	require.NotNil(t, fn)
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
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0, 1},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_input")
	require.NotNil(t, fn)
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
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_params")
	require.NotNil(t, fn)
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
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0, 1},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_params")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, ParamsReadError, res[0])
}

func Test_buildHostAPI_SetStateFromParams_Nil(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: nil,
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_params")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, ParamsReadError, res[0])
}

func Test_buildHostAPI_GetState(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	require.NoError(t, storage.Write(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0}))
	fn := hm.ExportedFunction("get_state")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, 6, res[0])
}

func Test_buildHostAPI_GetState_Missing(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("get_state")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateReadError, res[0])
}

func Test_buildHostAPI_GetState_TooLong(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	require.NoError(t, storage.Write(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0, 1}))
	fn := hm.ExportedFunction("get_state")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateReadError, res[0])
}

func Test_buildHostAPI_SetStateFromParamsDBWriteFails(t *testing.T) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	defer func() { _ = rt.Close(ctx) }()
	storage := NewAlwaysFailsMemoryStorage()
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	_, err = buildHostModule(ctx, rt, eCtx, storage)
	require.NoError(t, err)
	hm, err := rt.Instantiate(ctx, wasm)
	require.NoError(t, err)
	fn := hm.ExportedFunction("set_state_params")
	require.NotNil(t, fn)
	res, err := fn.Call(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, StateWriteError, res[0])
}
