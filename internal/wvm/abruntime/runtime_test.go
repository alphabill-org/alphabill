package abruntime

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/wvm"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type TestExecutionCtx struct {
	id        string
	src       []byte
	inputData []byte
	params    []byte
	txHash    []byte
}

func loadWasmFile(t *testing.T, path string) []byte {
	t.Helper()
	wasm, err := os.ReadFile(path)
	require.NoError(t, err)
	return wasm
}

func (t TestExecutionCtx) GetProgramID() *uint256.Int {
	id := sha256.Sum256([]byte(t.id))
	return uint256.NewInt(0).SetBytes(id[:])
}

func (t TestExecutionCtx) Wasm() []byte {
	return t.src
}

func (t TestExecutionCtx) GetInputData() []byte {
	return t.inputData
}

func (t TestExecutionCtx) GetParams() []byte {
	return t.params
}

func (t TestExecutionCtx) GetTxHash() []byte {
	return t.txHash
}

func TestCheckProgram(t *testing.T) {
	type args struct {
		ctx     context.Context
		src     []byte
		execCtx wvm.ExecutionCtx
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "wasm src is nil",
			args: args{
				ctx: context.Background(),
				execCtx: TestExecutionCtx{
					id:  "test",
					src: nil,
				},
			},
			wantErrStr: "wasm vm init error, wasm src is missing",
		},
		{
			name: "wasm src is empty",
			args: args{
				ctx: context.Background(),
				src: []byte{},
				execCtx: TestExecutionCtx{
					id: "test",
				},
			},
			wantErrStr: "wasm vm init error, wasm src is missing",
		},
		{
			name: "wasm src is invalid",
			args: args{
				ctx: context.Background(),
				src: []byte{1, 2, 3, 4},
				execCtx: TestExecutionCtx{
					id: "test",
				},
			},
			wantErrStr: "wasm vm init error, failed to initiate VM with wasm source, invalid magic number",
		},
		{
			name: "empty wasm module, no public API calls",
			args: args{
				ctx: context.Background(),
				src: loadWasmFile(t, "../testdata/empty.wasm"),
				execCtx: TestExecutionCtx{
					id: "test",
				},
			},
			wantErrStr: "wasm program error, no exported functions",
		},
		{
			name: "missing host API call",
			args: args{
				ctx: context.Background(),
				src: loadWasmFile(t, "../testdata/invalid_api.wasm"),
				execCtx: TestExecutionCtx{
					id: "test",
				},
			},
			wantErrStr: "wasm vm init error, failed to initiate VM with wasm source, \"get_test_missing\" is not exported in module \"ab\"",
		},
		{
			name: "missing host module ab_v0",
			args: args{
				ctx: context.Background(),
				src: loadWasmFile(t, "../testdata/missing_module.wasm"),
				execCtx: TestExecutionCtx{
					id: "test",
				},
			},
			wantErrStr: "wasm vm init error, failed to initiate VM with wasm source, module[ab_v0] not instantiated",
		},
		{
			name: "counter ok",
			args: args{
				ctx: context.Background(),
				src: loadWasmFile(t, "../testdata/counter.wasm"),
				execCtx: TestExecutionCtx{
					id: "counter",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckProgram(tt.args.ctx, tt.args.src, tt.args.execCtx)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCall(t *testing.T) {
	type args struct {
		ctx     context.Context
		src     []byte
		fName   string
		execCtx wvm.ExecutionCtx
		storage wvm.Storage
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "counter-ok",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "count",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: []byte{1, 0, 0, 0, 0, 0, 0, 0},
					params:    []byte{1, 0, 0, 0, 0, 0, 0, 0},
				},
				storage: wvm.NewMemoryStorage(),
			},
		},
		{
			name: "counter invalid input nil",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "count",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: nil,
					params:    []byte{1, 0, 0, 0, 0, 0, 0, 0},
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call exited with error, program exited with error 4",
		},
		{
			name: "counter invalid input",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "count",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: []byte{1},
					params:    []byte{1, 0, 0, 0, 0, 0, 0, 0},
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call exited with error, program exited with error 4",
		},
		{
			name: "counter invalid params nil",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "count",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: []byte{1, 0, 0, 0, 0, 0, 0, 0},
					params:    nil,
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call exited with error, program exited with error 3",
		},
		{
			name: "counter params invalid",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "count",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: []byte{1, 0, 0, 0, 0, 0, 0, 0},
					params:    []byte{1},
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call exited with error, program exited with error 3",
		},
		{
			name: "counter-invalid-function",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/counter.wasm"),
				fName: "countt",
				execCtx: TestExecutionCtx{
					id:        "test",
					inputData: []byte{1, 0, 0, 0, 0, 0, 0, 0},
					params:    []byte{1, 0, 0, 0, 0, 0, 0, 0},
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call failed, function countt not found",
		},
		{
			name: "nok",
			args: args{
				ctx:   context.Background(),
				src:   loadWasmFile(t, "../testdata/empty.wasm"),
				fName: "whatever",
				execCtx: TestExecutionCtx{
					id: "test",
				},
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "program call failed, function whatever not found",
		},
		{
			name: "invalid wasm",
			args: args{
				ctx: context.Background(),
				src: []byte{3, 69, 7},
				execCtx: TestExecutionCtx{
					id: "test",
				},
				fName:   "whatever",
				storage: wvm.NewMemoryStorage(),
			},
			wantErrStr: "wasm program load failed, failed to initiate VM with wasm source, invalid magic number",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Call(tt.args.ctx, tt.args.src, tt.args.fName, tt.args.execCtx, tt.args.storage)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
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
			args: args{retVal: []uint64{wvm.Success}},
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
			err := validateResult(tt.args.retVal)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
