package wvm

import (
	"context"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb"
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

// Todo: write tests for host functionality
