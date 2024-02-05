package wvm

import (
	"bytes"
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/tetratelabs/wazero/api"
)

const (
	Success = 0
	Error   = -1
)

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
	fmt.Println("ptr", ptr, "size", size)
	data, ok := m.Memory().Read(ptr, size)
	if !ok {
		panic("write overflow")
	}
	return data
}

func storageReadV1(ctx context.Context, m api.Module, fileID uint32) uint64 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	rtCtx.Log.DebugContext(ctx, fmt.Sprintf("program state file request, %v", fileID))
	var Value []byte
	found, err := rtCtx.Storage.Read(util.Uint32ToBytes(fileID), &Value)
	if !found {
		return 0
	}
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "get state from storage failed, %v", logger.Error(err))
		return 0
	}
	dataLen := uint32(len(Value))
	offset, err := rtCtx.Alloc.Allocate(m.Memory(), dataLen)
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "program state file memory allocation failed failed: %w", err)
	}
	if ok := m.Memory().Write(offset, Value); !ok {
		rtCtx.Log.WarnContext(ctx, "program state file write failed")
		return 0
	}
	return newPointerSize(offset, dataLen)
}

func storageWriteV1(ctx context.Context, m api.Module, fileID uint32, value uint64) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}
	prt, size := splitPointerSize(value)
	data, ok := m.Memory().Read(prt, size)
	if !ok {
		rtCtx.Log.WarnContext(ctx, "failed to read state from program memory")
		return -1
	}
	rtCtx.Log.WarnContext(ctx, fmt.Sprintf("set state, %v id, new state: %v", fileID, data))
	if err := rtCtx.Storage.Write(util.Uint32ToBytes(fileID), data); err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to persist program state")
		return -1
	}
	return 0
}

func p2pkhV1(ctx context.Context, m api.Module) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	p2pkh256Signature := &templates.P2pkh256Signature{}
	if err := cbor.Unmarshal(rtCtx.AbCtx.Txo.OwnerProof, p2pkh256Signature); err != nil {
		rtCtx.Log.WarnContext(ctx, "p2pkh v1 failed to decode P2PKH256 signature:", logger.Error(err))
		return -1
	}
	pubKeyHash := rtCtx.AbCtx.InitArgs
	if len(pubKeyHash) != 32 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid pubkey hash size: %X, expected 32", pubKeyHash))
		return -1
	}
	if len(p2pkh256Signature.Sig) != 65 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid signature size: %X, expected 65", p2pkh256Signature.Sig))
		return -1
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid pubkey size: %X, expected 33", p2pkh256Signature.PubKey))
		return -1
	}
	if !bytes.Equal(pubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
		rtCtx.Log.WarnContext(ctx, "pubkey hash does not match")
		return -1
	}
	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to create verifier:", logger.Error(err))
		return -1
	}
	payloadBytes, err := rtCtx.AbCtx.Txo.PayloadBytes()
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to marshal payload bytes: %w", logger.Error(err))
		return -1
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, payloadBytes); err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to verify signature: ", logger.Error(err))
		return -1
	}
	return 1
}

func p2pkhV2(ctx context.Context, m api.Module, pubKeyHashPtr uint64) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	p2pkh256Signature := &templates.P2pkh256Signature{}
	if err := cbor.Unmarshal(rtCtx.AbCtx.Txo.OwnerProof, p2pkh256Signature); err != nil {
		rtCtx.Log.WarnContext(ctx, "p2pkh v1 failed to decode P2PKH256 signature:", logger.Error(err))
		return -1
	}
	pubKeyHash := read(m, pubKeyHashPtr)
	if len(pubKeyHash) != 32 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid pubkey hash size: %X, expected 32", pubKeyHash))
		return -1
	}
	if len(p2pkh256Signature.Sig) != 65 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid signature size: %X, expected 65", p2pkh256Signature.Sig))
		return -1
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid pubkey size: %X, expected 33", p2pkh256Signature.PubKey))
		return -1
	}
	if !bytes.Equal(pubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
		rtCtx.Log.WarnContext(ctx, "pubkey hash does not match")
		return -1
	}
	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to create verifier:", logger.Error(err))
		return -1
	}
	payloadBytes, err := rtCtx.AbCtx.Txo.PayloadBytes()
	if err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to marshal payload bytes: %w", logger.Error(err))
		return -1
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, payloadBytes); err != nil {
		rtCtx.Log.WarnContext(ctx, "failed to verify signature: ", logger.Error(err))
		return -1
	}
	return 1
}

func loggingV1(ctx context.Context, m api.Module, level uint32, msgData uint64) {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}
	msg := string(read(m, msgData))
	switch level {
	case 0:
		rtCtx.Log.ErrorContext(ctx, msg)
	case 1:
		rtCtx.Log.WarnContext(ctx, msg)
	case 2:
		rtCtx.Log.InfoContext(ctx, msg)
	case 3:
		rtCtx.Log.DebugContext(ctx, msg)
	default:
		rtCtx.Log.ErrorContext(ctx, fmt.Sprintf("unknown leved %v: %s", level, msg))
	}
}

func extFree(ctx context.Context, m api.Module, addr uint32) {
	allocator := ctx.Value(runtimeContextKey).(*VmContext).Alloc

	// Deallocate memory
	err := allocator.Deallocate(m.Memory(), addr)
	if err != nil {
		panic(err)
	}
}

func extMalloc(ctx context.Context, m api.Module, size uint32) uint32 {
	allocator := ctx.Value(runtimeContextKey).(*VmContext).Alloc

	// Allocate memory
	res, err := allocator.Allocate(m.Memory(), size)
	if err != nil {
		panic(err)
	}

	return res
}
