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

func getStateV1(ctx context.Context, m api.Module, fileID, offset, size uint32) int32 {
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
		return -1
	}
	if uint32(len(Value)) > size {
		rtCtx.Log.WarnContext(ctx, "program state file is too big")
		return -2
	}
	if ok := m.Memory().Write(offset, Value); !ok {
		rtCtx.Log.WarnContext(ctx, "program state file write failed")
		return -1
	}
	return int32(len(Value))
}

func setStateV1(ctx context.Context, m api.Module, fileID, offset, size uint32) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	data, ok := m.Memory().Read(offset, size)
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

func getInitValueV1(ctx context.Context, m api.Module, offset, size uint32) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	rtCtx.Log.WarnContext(ctx, "get program parameters data")
	params := rtCtx.AbCtx.InitArgs
	if params == nil {
		return -2
	}
	if uint32(len(params)) > size {
		rtCtx.Log.WarnContext(ctx, "program parameters too big")
		return -2
	}
	if ok := m.Memory().Write(offset, params); !ok {
		rtCtx.Log.WarnContext(ctx, "program parameters write failed")
		return -3
	}
	return int32(len(params))
}

func getInputData(ctx context.Context, m api.Module, offset, size uint32) int32 {
	rtCtx := ctx.Value(runtimeContextKey).(*VmContext)
	if rtCtx == nil {
		panic("nil runtime context")
	}

	rtCtx.Log.WarnContext(ctx, "get input data")
	input := rtCtx.InputArgs
	if input == nil {
		return -2
	}
	if uint32(len(input)) > size {
		rtCtx.Log.WarnContext(ctx, "input data is too big")
		return -2
	}
	if ok := m.Memory().Write(offset, input); !ok {
		rtCtx.Log.WarnContext(ctx, "input data write failed")
		return -1
	}
	return int32(len(input))
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
	pubKeyHas := rtCtx.AbCtx.InitArgs
	if len(pubKeyHas) != 32 {
		rtCtx.Log.WarnContext(ctx, fmt.Sprintf("invalid pubkey hash size: %X, expected 32", pubKeyHas))
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
	if !bytes.Equal(pubKeyHas, hash.Sum256(p2pkh256Signature.PubKey)) {
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
