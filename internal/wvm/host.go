package wvm

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	abModule           = "ab"
	getState           = "get_state"
	setState           = "set_state"
	getParams          = "get_params"
	getInputParameters = "get_input_params"
)

func buildHostModule(ctx context.Context, rt wazero.Runtime, eCtx ExecutionCtx, storage Storage) (api.Module, error) {
	return rt.NewHostModuleBuilder(abModule).
		NewFunctionBuilder().WithGoModuleFunction(buildGetStateHostFn(ctx, storage), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export(getState).
		NewFunctionBuilder().WithGoModuleFunction(buildSetStateHostFn(ctx, storage), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export(setState).
		NewFunctionBuilder().WithGoModuleFunction(buildGetParamsHostFn(ctx, eCtx), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export(getParams).
		NewFunctionBuilder().WithGoModuleFunction(buildGetInputParametersFn(ctx, eCtx), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export(getInputParameters).
		Instantiate(ctx)
}

func buildGetStateHostFn(_ context.Context, storage Storage) api.GoModuleFunc {
	return func(ctx context.Context, m api.Module, stack []uint64) {
		fileID := api.DecodeU32(stack[0]) // read the parameter from the stack
		offset := api.DecodeU32(stack[1]) // read the parameter from the stack
		size := api.DecodeU32(stack[2])   // read the parameter from the stack
		logger.Debug("program state file request, %v", fileID)
		file, err := storage.Read(util.Uint32ToBytes(fileID))
		if err != nil {
			logger.Warning("get state from system failed, %v", err)
			stack[0] = api.EncodeI32(-1)
			return
		}
		if uint32(len(file)) > size {
			logger.Warning("program state file is too big")
			stack[0] = api.EncodeI32(-2)
			return
		}
		if ok := m.Memory().Write(offset, file); !ok {
			logger.Warning("program state file write failed")
			stack[0] = api.EncodeI32(-1)
			return
		}
		stack[0] = api.EncodeI32(int32(len(file)))
	}
}

func buildSetStateHostFn(_ context.Context, storage Storage) api.GoModuleFunc {
	return func(ctx context.Context, m api.Module, stack []uint64) {
		fileID := api.DecodeU32(stack[0]) // read the parameter from the stack
		offset := api.DecodeU32(stack[1]) // read the parameter from the stack
		size := api.DecodeU32(stack[2])   // read the parameter from the stack
		fBytes, ok := m.Memory().Read(offset, size)
		if !ok {
			logger.Warning("failed to read state from program memory")
			stack[0] = api.EncodeI32(-1)
			return
		}
		logger.Debug("set state, %v id, new state: %v", fileID, fBytes)
		if err := storage.Write(util.Uint32ToBytes(fileID), fBytes); err != nil {
			logger.Warning("failed to persist program file")
			stack[0] = api.EncodeI32(-1)
			return
		}
		stack[0] = api.EncodeI32(0)
	}
}

func buildGetParamsHostFn(_ context.Context, eCtx ExecutionCtx) api.GoModuleFunc {
	return func(ctx context.Context, m api.Module, stack []uint64) {
		offset := api.DecodeU32(stack[0])
		size := api.DecodeU32(stack[1])
		logger.Debug("get program parameters data")
		params := eCtx.GetParams()
		if params == nil {
			stack[0] = api.EncodeI32(-2)
			return
		}
		if uint32(len(params)) > size {
			logger.Warning("program parameters too big")
			stack[0] = api.EncodeI32(-2)
			return
		}
		if ok := m.Memory().Write(offset, params); !ok {
			logger.Warning("program parameters write failed")
			stack[0] = api.EncodeI32(-1)
			return
		}
		stack[0] = api.EncodeI32(int32(len(params)))
	}
}

func buildGetInputParametersFn(_ context.Context, eCtx ExecutionCtx) api.GoModuleFunc {
	return func(ctx context.Context, m api.Module, stack []uint64) {
		offset := api.DecodeU32(stack[0])
		size := api.DecodeU32(stack[1])
		logger.Debug("get input data")
		input := eCtx.GetInputData()
		if input == nil {
			stack[0] = api.EncodeI32(-2)
			return
		}
		if uint32(len(input)) > size {
			logger.Warning("inout data is too big")
			stack[0] = api.EncodeI32(-2)
			return
		}
		if ok := m.Memory().Write(offset, input); !ok {
			logger.Warning("input data write failed")
			stack[0] = api.EncodeI32(-1)
			return
		}
		stack[0] = api.EncodeI32(int32(len(input)))
		return
	}
}
