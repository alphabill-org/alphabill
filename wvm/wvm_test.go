package wvm

import (
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	testlogr "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/empty.wasm
var emptyWasm []byte

//go:embed testdata/add_one.wasm
var addWasm []byte

//go:embed testdata/invalid_api.wasm
var invalidAPIWasm []byte

//go:embed testdata/p2pkh_v1/p2pkh.wasm
var p2pkhV1Wasm []byte

//go:embed testdata/p2pkh_v2_c/build/p2pkh_v2.wasm
var p2pkhV2Wasm []byte

func TestNewNoHostModules(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)
	wvm, err := New(ctx, addWasm, nil, obs.Logger())
	require.NoError(t, err)
	require.NotNil(t, wvm)
	fn, err := wvm.getApiFn("add_one")
	require.NoError(t, err)
	require.NotNil(t, fn)
	res, err := fn.Call(ctx, uint64(3))
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualValues(t, 4, res[0])
}

func TestNew_RequiresABModuleAndFails(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)
	wvm, err := New(ctx, invalidAPIWasm, nil, obs.Logger())
	require.EqualError(t, err, "failed to initiate VM with wasm source, \"get_test_missing\" is not exported in module \"ab\"")
	require.Nil(t, wvm)
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)

	memDB, err := memorydb.New()
	require.NoError(t, err)
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)
	abCtx := &AbContext{
		InitArgs: make([]byte, 8),
	}
	require.NoError(t, err)
	wvm, err := New(ctx, addWasm, abCtx, obs.Logger(), WithStorage(memDB))
	require.NoError(t, err)
	require.NotNil(t, wvm)
}

func TestWasmVM_CheckApiCallExists_OK(t *testing.T) {
	ctx := context.Background()
	memDB, err := memorydb.New()
	require.NoError(t, err)
	input := make([]byte, 8)
	binary.LittleEndian.PutUint64(input, 1)

	abCtx := &AbContext{
		InitArgs: make([]byte, 8),
	}
	obs := observability.Default(t)
	wvm, err := New(ctx, addWasm, abCtx, obs.Logger(), WithStorage(memDB))
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
}

// loads src that does not export any functions
func TestWasmVM_CheckApiCallExists_NOK(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)
	wvm, err := New(ctx, emptyWasm, nil, obs.Logger())
	require.NoError(t, err)
	require.ErrorContains(t, wvm.CheckApiCallExists(), "no exported functions")
}

func TestPredicate_P2PKH_V1(t *testing.T) {
	ctx := context.Background()
	payload := &types.Payload{
		SystemID: types.SystemID(1),
		Type:     money.PayloadTypeTransfer,
		UnitID:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
	}
	bytes, err := payload.Bytes()
	require.NoError(t, err)
	sig, pubKey := testsig.SignBytes(t, bytes)
	require.NoError(t, err)
	execCtx := &AbContext{
		InitArgs: hash.Sum256(pubKey),
		Txo: &types.TransactionOrder{
			Payload:    payload,
			OwnerProof: templates.NewP2pkh256SignatureBytes(sig, pubKey)},
	}
	obs := observability.Default(t)
	wvm, err := New(ctx, p2pkhV1Wasm, execCtx, obs.Logger())
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
	res, err := wvm.Exec(ctx, "run")
	require.NoError(t, err)
	require.True(t, res)
}

func TestPredicate_P2PKH_V2(t *testing.T) {
	ctx := context.Background()
	payload := &types.Payload{
		SystemID: types.SystemID(1),
		Type:     money.PayloadTypeTransfer,
		UnitID:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
	}
	bytes, err := payload.Bytes()
	require.NoError(t, err)
	sig, pubKey := testsig.SignBytes(t, bytes)
	require.NoError(t, err)
	execCtx := &AbContext{
		Txo: &types.TransactionOrder{
			Payload:    payload,
			OwnerProof: templates.NewP2pkh256SignatureBytes(sig, pubKey)},
	}
	obs := observability.Default(t)
	wvm, err := New(ctx, p2pkhV2Wasm, execCtx, obs.Logger())
	require.NoError(t, err)
	require.NoError(t, wvm.CheckApiCallExists())
	pubKeyHash := hash.Sum256(pubKey)
	obs.Logger().InfoContext(ctx, fmt.Sprintf("Send PubKey Hash: %x", pubKeyHash))
	res, err := wvm.Exec(ctx, "run", pubKeyHash)
	require.NoError(t, err)
	require.True(t, res)
}

func Benchmark_runPredicate(b *testing.B) {
	b.StopTimer()
	ctx := context.Background()
	logF := testlogr.LoggerBuilder(b)
	logger, err := logF(&logger.LogConfiguration{})
	if err != nil {
		b.Fatal(err)
	}
	payload := &types.Payload{
		SystemID: types.SystemID(1),
		Type:     money.PayloadTypeTransfer,
		UnitID:   []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
	}
	pubKey := []byte{0x2, 0x12, 0x91, 0x1c, 0x73, 0x41, 0x39, 0x9e, 0x87, 0x68, 0x0, 0xa2, 0x68, 0x85, 0x5c, 0x89, 0x4c, 0x43, 0xeb, 0x84, 0x9a, 0x72, 0xac, 0x5a, 0x9d, 0x26, 0xa0, 0x9, 0x10, 0x41, 0xc1, 0x7, 0xf0}
	privKey := []byte{0xa5, 0xe8, 0xbf, 0xf9, 0x73, 0x3e, 0xbc, 0x75, 0x1a, 0x45, 0xca, 0x4b, 0x8c, 0xc6, 0xce, 0x8e, 0x76, 0xc8, 0x31, 0x6a, 0x5e, 0xb5, 0x56, 0xf7, 0x38, 0x9, 0x2d, 0xf6, 0x23, 0x2e, 0x78, 0xde}

	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		b.Fatal(err)
	}
	buf, err := payload.Bytes()
	if err != nil {
		b.Fatal(err)
	}
	p2hs := predicates.P2pkh256Signature{PubKey: pubKey}
	if p2hs.Sig, err = signer.SignBytes(buf); err != nil {
		b.Fatal(err)
	}

	if buf, err = cbor.Marshal(p2hs); err != nil {
		b.Fatal(err)
	}
	execCtx := &AbContext{
		InitArgs: hash.Sum256(pubKey),
		Txo: &types.TransactionOrder{
			Payload:    payload,
			OwnerProof: buf},
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		wvm, err := New(ctx, p2pkhV1Wasm, execCtx, logger)
		if err != nil {
			b.Fatal(err)
		}
		res, err := wvm.Exec(ctx, "run")
		if err != nil {
			b.Fatal(err)
		}
		if !res {
			b.Fatal("expected predicate to eval to true")
		}
	}
}
