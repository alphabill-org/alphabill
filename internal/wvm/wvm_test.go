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
