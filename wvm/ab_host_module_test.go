package wvm

import (
	"context"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

const stateID = 0xaabbccdd

func initWvmHostTest(t *testing.T, ctx context.Context, args []byte, s keyvaluedb.KeyValueDB) *WasmVM {
	wasm, err := os.ReadFile("testdata/host_test.wasm")
	require.NoError(t, err)
	obs := observability.Default(t)
	abCtx := &AbContext{
		SysID:    1,
		Round:    2,
		Txo:      nil,
		InitArgs: args,
	}
	// init WVM
	wvm, err := New(ctx, wasm, abCtx, obs.Logger(), WithStorage(s))
	require.NoError(t, err)
	return wvm
}

func Test_buildHostAPI_SetStateFromInput(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_input", []byte{1, 0, 0, 0, 0, 0, 0, 0})
	require.NoError(t, err)
	require.True(t, res)
	// verify state was written to db
	var state []byte
	f, err := storage.Read(util.Uint32ToBytes(stateID), &state)
	require.NoError(t, err)
	require.True(t, f)
	require.EqualValues(t, state, []byte{1, 0, 0, 0, 0, 0, 0, 0})
}

func Test_buildHostAPI_SetStateFromInput_Nil(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_input", nil)
	require.NoError(t, err)
	require.Len(t, res, 1)
	// verify state was written to db
	var state []byte
	f, err := storage.Read(util.Uint32ToBytes(stateID), &state)
	require.True(t, f)
	require.Error(t, err)
	require.EqualValues(t, state, []byte{1, 0, 0, 0, 0, 0, 0, 0})
}

func Test_buildHostAPI_SetStateFromInput_Empty(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_input", nil)
	require.Error(t, err)
	require.Nil(t, res)
	// verify state was written not written to db
	var state []byte
	val, err := storage.Read(util.Uint32ToBytes(stateID), &state)
	require.Error(t, err)
	require.Nil(t, val)
}

func Test_buildHostAPI_SetStateFromInput_TooLong(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_input", []byte{1, 0, 0, 0, 0, 0, 0, 0, 1})
	require.Error(t, err)
	require.Nil(t, res)
}

func Test_buildHostAPI_SetStateFromParams(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_params", []byte{1, 0, 0, 0, 0, 0, 0, 0})
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.True(t, res)
	// verify state was written to db
	var state []byte
	f, err := storage.Read(util.Uint32ToBytes(stateID), &state)
	require.NoError(t, err)
	require.True(t, f)
	require.EqualValues(t, initArgs, state)
}

func Test_buildHostAPI_SetStateFromParams_TooBig(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "set_state_params", []byte{1, 0, 0, 0, 0, 0, 0, 0})
	require.NoError(t, err)
	require.True(t, res)
}

func Test_buildHostAPI_SetStateFromParams_Nil(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	wvm := initWvmHostTest(t, ctx, nil, storage)
	res, err := wvm.Exec(ctx, "set_state_params", []byte{1, 0, 0, 0, 0, 0, 0, 0})
	require.Error(t, err)
	require.False(t, res)
}

func Test_buildHostAPI_GetState(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storage.Write(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0}))
	initArgs := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	wvm := initWvmHostTest(t, ctx, initArgs, storage)
	res, err := wvm.Exec(ctx, "get_state", nil)
	require.NoError(t, err)
	require.True(t, res)
}

/*
func Test_buildHostAPI_GetState_Missing(t *testing.T) {
	ctx := context.Background()
	storage, err := memorydb.New()
	require.NoError(t, err)
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
	storage, err := memorydb.New()
	require.NoError(t, err)
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	require.NoError(t, storage.Put(util.Uint32ToBytes(stateID), []byte{6, 0, 0, 0, 0, 0, 0, 0, 1}))
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
	storage, err := memorydb.New()
	require.NoError(t, err)
	storage.MockWriteError(fmt.Errorf("mock db write failed"))
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
*/
