package vm

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const WasmSuccess = 0

type WasmVM struct {
	runtime wazero.Runtime
	mod     api.Module
}

type ExecutionCtx interface {
	GetProgramID() *uint256.Int
	Wasm() []byte
	GetInputData() []byte
	GetParams() []byte
	GetTxHash() []byte
}

type Storage interface {
	Read(key []byte) ([]byte, error)
	Write(key []byte, file []byte) error
}

func New(ctx context.Context, execCtx ExecutionCtx, opts ...Option) (*WasmVM, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.storage == nil {
		return nil, fmt.Errorf("storage is nil")
	}
	rt := wazero.NewRuntimeWithConfig(ctx, options.cfg)
	if err := addHostFunctions(ctx, rt, options.storage, execCtx); err != nil {
		return nil, fmt.Errorf("host iterface init failed, %w", err)
	}
	m, err := rt.Instantiate(ctx, execCtx.Wasm())
	if err != nil {
		return nil, fmt.Errorf("failed to initiate VM with wasm source, %w", err)
	}
	return &WasmVM{
		runtime: rt,
		mod:     m,
	}, nil
}

func (vm *WasmVM) CheckApiCallExists() error {
	if len(vm.mod.ExportedFunctionDefinitions()) < 1 {
		return fmt.Errorf("no exported functions")
	}
	return nil
}

func (vm *WasmVM) GetApiFn(fnName string) (api.Function, error) {
	fn := vm.mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("function %v not found", fnName)
	}
	return fn, nil
}

func addHostFunctions(ctx context.Context, rt wazero.Runtime, storage Storage, execCtx ExecutionCtx) error {
	// Dummy test for host defined functions
	_, err := rt.NewHostModuleBuilder("ab_v0").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, fileID, offset, size uint32) int32 {
		logger.Debug("state request, %v", fileID)
		file, err := storage.Read(util.Uint32ToBytes(fileID))
		if err != nil {
			logger.Warning("get state from system failed, %v", err)
			return -1
		}
		if uint32(len(file)) > size {
			logger.Warning("state too big")
			return -2
		}
		if ok := m.Memory().Write(offset, file); !ok {
			logger.Warning("state write failed")
			return -1
		}
		return int32(len(file))
	}).Export("get_state").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, fileID, offset, cnt uint32) int32 {
		fBytes, ok := m.Memory().Read(offset, cnt)
		if !ok {
			logger.Warning("failed to read state from program memory")
			return -1
		}
		logger.Debug("set state, %v id, new state: %v", fileID, fBytes)
		if err := storage.Write(util.Uint32ToBytes(fileID), fBytes); err != nil {
			logger.Warning("failed to persist program file")
			return -1
		}
		return 0
	}).Export("set_state").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, offset, size uint32) int32 {
		logger.Debug("get init data")
		initData := execCtx.GetParams()
		if uint32(len(initData)) > size {
			logger.Warning("state too big")
			return -2
		}
		if ok := m.Memory().Write(offset, initData); !ok {
			logger.Warning("state write failed")
			return -1
		}
		return int32(len(initData))
	}).Export("get_params").
		NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module, offset, size uint32) int32 {
		logger.Debug("get input data")
		input := execCtx.GetInputData()

		if uint32(len(input)) > size {
			logger.Warning("state too big")
			return -2
		}
		if ok := m.Memory().Write(offset, input); !ok {
			logger.Warning("state write failed")
			return -1
		}
		return int32(len(input))
	}).Export("get_input_params").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("host functions instanciate failed")
	}
	return nil
}

func ValidateResult(retVal []uint64) error {
	if len(retVal) > 1 {
		return fmt.Errorf("unexpected return value length %v", len(retVal))
	}
	// todo: translate error code to go error
	if retVal[0] != WasmSuccess {
		return fmt.Errorf("program exited with error %v", retVal[0])
	}
	return nil
}
