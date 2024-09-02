package wvm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/logger"
)

/*
addHostModule adds "host" module to the "rt".
The host module provides "utility APIs" for the runtime, ie memory manager and logging.
*/
func addHostModule(ctx context.Context, rt wazero.Runtime, observe Observability) error {
	_, err := rt.NewHostModuleBuilder("host").
		NewFunctionBuilder().WithFunc(logMsg).Export("log_msg").
		NewFunctionBuilder().WithGoModuleFunction(extMalloc(observe), []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export("ext_malloc").
		NewFunctionBuilder().WithGoModuleFunction(extFree(observe), []api.ValueType{api.ValueTypeI32}, []api.ValueType{}).Export("ext_free").
		// do not expose storage API, need to decide do we want to support it, fees etc...
		//NewFunctionBuilder().WithFunc(storageReadV1).Export("storage_read").
		//NewFunctionBuilder().WithFunc(storageWriteV1).Export("storage_write").
		Instantiate(ctx)
	return err
}

// toPointerSize converts an uint32 pointer and uint32 size
// to an int64 pointer size.
func newPointerSize(ptr, size uint32) (pointerSize uint64) {
	return uint64(ptr) | (uint64(size) << 32)
}

// splitPointerSize converts a 64bit pointer size to an
// uint32 pointer and a uint32 size.
func splitPointerSize(pointerSize uint64) (ptr, size uint32) {
	return uint32(pointerSize), uint32(pointerSize >> 32)
}

// read will read from 64 bit pointer size and return a byte slice
func read(m api.Module, pointerSize uint64) (data []byte) {
	ptr, size := splitPointerSize(pointerSize)
	data, ok := m.Memory().Read(ptr, size)
	if !ok {
		panic("out of range read from shared memory")
	}
	return data
}

func storageReadV1(ctx context.Context, m api.Module, fileID uint32) uint64 {
	rtCtx := extractVMContext(ctx)
	rtCtx.log.DebugContext(ctx, fmt.Sprintf("program state file request, %v", fileID))
	var Value []byte
	found, err := rtCtx.storage.Read(util.Uint32ToBytes(fileID), &Value)
	if !found {
		return 0
	}
	if err != nil {
		rtCtx.log.WarnContext(ctx, "get state from storage failed, %v", logger.Error(err))
		return 0
	}
	dataLen := uint32(len(Value))
	offset, err := rtCtx.memMngr.Alloc(m.Memory(), dataLen)
	if err != nil {
		rtCtx.log.WarnContext(ctx, "program state file memory allocation failed failed", logger.Error(err))
	}
	if ok := m.Memory().Write(offset, Value); !ok {
		rtCtx.log.WarnContext(ctx, "program state file write failed")
		return 0
	}
	return newPointerSize(offset, dataLen)
}

func storageWriteV1(ctx context.Context, m api.Module, fileID uint32, value uint64) int32 {
	rtCtx := extractVMContext(ctx)
	prt, size := splitPointerSize(value)
	data, ok := m.Memory().Read(prt, size)
	if !ok {
		rtCtx.log.WarnContext(ctx, "failed to read state from program memory")
		return -1
	}
	rtCtx.log.WarnContext(ctx, fmt.Sprintf("set state, %v id, new state: %v", fileID, data))
	if err := rtCtx.storage.Write(util.Uint32ToBytes(fileID), data); err != nil {
		rtCtx.log.WarnContext(ctx, "failed to persist program state")
		return -1
	}
	return 0
}

func logMsg(ctx context.Context, m api.Module, level uint32, msgData uint64) {
	rtCtx := extractVMContext(ctx)
	msg := read(m, msgData)
	switch level {
	case 0:
		rtCtx.log.ErrorContext(ctx, string(msg))
	case 1:
		rtCtx.log.WarnContext(ctx, string(msg))
	case 2:
		rtCtx.log.InfoContext(ctx, string(msg))
	case 3:
		rtCtx.log.DebugContext(ctx, string(msg))
	default:
		rtCtx.log.ErrorContext(ctx, fmt.Sprintf("unknown level %v: %s", level, msg))
	}
}

func extFree(_ Observability) api.GoModuleFunc {
	//log := observe.Logger()
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		addr := api.DecodeU32(stack[0])
		//log.DebugContext(ctx, fmt.Sprintf("%s.Free(%d)", mod.Name(), addr))
		allocator := ctx.Value(runtimeContextKey).(*vmContext).memMngr

		if err := allocator.Free(mod.Memory(), addr); err != nil {
			panic(err)
		}
	}
}

func extMalloc(_ Observability) api.GoModuleFunc {
	//log := observe.Logger()
	return func(ctx context.Context, mod api.Module, stack []uint64) {
		allocator := ctx.Value(runtimeContextKey).(*vmContext).memMngr

		// Allocate memory
		size := api.DecodeU32(stack[0])
		res, err := allocator.Alloc(mod.Memory(), size)
		if err != nil {
			panic(err)
		}
		//log.DebugContext(ctx, fmt.Sprintf("%s.Alloc(%d) => %d", mod.Name(), size, res))

		stack[0] = api.EncodeU32(res)
	}
}
