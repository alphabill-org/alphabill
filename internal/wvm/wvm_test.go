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

type TestExecCtx struct {
	id     string
	wasm   []byte
	input  []byte
	params []byte
	txHash []byte
}

func (t TestExecCtx) GetProgramID() *uint256.Int {
	return uint256.NewInt(0).SetBytes([]byte(t.id))
}

func (t TestExecCtx) Wasm() []byte {
	return t.wasm
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

func TestNew(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)
	execCtx := &TestExecCtx{
		id:     "counter",
		wasm:   counterWasm,
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	wvm, err := New(ctx, execCtx, WithStorage(storage))
	require.NoError(t, err)
	require.NotNil(t, wvm)
}

func TestNew_StorageNil(t *testing.T) {
	ctx := context.Background()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)
	execCtx := &TestExecCtx{
		id:     "counter",
		wasm:   counterWasm,
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	wvm, err := New(ctx, execCtx, WithStorage(nil))
	require.ErrorContains(t, err, "storage is nil")
	require.Nil(t, wvm)
}

func TestValidateResult(t *testing.T) {
	type args struct {
		retVal []uint64
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "ok",
			args: args{retVal: []uint64{WasmSuccess}},
		},
		{
			name:       "unexpected number of return values",
			args:       args{retVal: []uint64{1, 2}},
			wantErrStr: "unexpected return value length 2",
		},
		{
			name:       "error code",
			args:       args{retVal: []uint64{2}},
			wantErrStr: "program exited with error 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResult(tt.args.retVal)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWasmVM_CheckApiCallExists_OK(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage()
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)

	execCtx := &TestExecCtx{
		id:     "counter",
		wasm:   counterWasm,
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	wvm, err := New(ctx, execCtx, WithStorage(storage))
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
		wasm:   emptyWasm,
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	wvm, err := New(ctx, execCtx, WithStorage(storage))
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
		wasm:   counterWasm,
		input:  input,
		params: make([]byte, 8),
		txHash: make([]byte, 32),
	}
	wvm, err := New(ctx, execCtx, WithStorage(storage))
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
	fn, err := wvm.GetApiFn("count")
	require.NoError(t, err)
	require.NotNil(t, fn)
	fn, err = wvm.GetApiFn("help")
	require.Error(t, err)
	require.Nil(t, fn)
}
