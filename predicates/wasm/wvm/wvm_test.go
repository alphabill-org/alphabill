package wvm

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/allocator"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	moneyenc "github.com/alphabill-org/alphabill/txsystem/money/encoder"
	tokenenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

//go:embed testdata/add_one/target/wasm32-unknown-unknown/release/add_one.wasm
var addOneWasm []byte

//go:embed testdata/missingHostAPI/target/wasm32-unknown-unknown/release/invalid_api.wasm
var invalidAPIWasm []byte

//go:embed testdata/p2pkh_v1/p2pkh.wasm
var p2pkhV1Wasm []byte

//go:embed testdata/conference_tickets/v1/conf_tickets.wasm
var ticketsWasm []byte

func Test_conference_tickets(t *testing.T) {

	// conference organizer
	signerOrg, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifierOrg, err := signerOrg.Verifier()
	require.NoError(t, err)
	pubKeyOrg, err := verifierOrg.MarshalPublicKey()
	require.NoError(t, err)
	// customer buying conference ticket
	signerAttendee, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifierAttendee, err := signerAttendee.Verifier()
	require.NoError(t, err)
	pubKeyAttendee, err := verifierAttendee.MarshalPublicKey()
	require.NoError(t, err)
	// not ideal but we use org and attendee also for trustbase
	// the testblock.CreateProof takes single signer as trustbase and used id "test" for it
	//trustbase := map[string]abcrypto.Verifier{"test": verifierAttendee, "attendee": verifierAttendee, "org": verifierOrg}
	// need VerifyQuorumSignatures for verifing tx proofs
	trustbase := &mockRootTrustBase{
		verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return nil, nil },
	}

	enc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(enc.RegisterAttrEncoder))
	require.NoError(t, tokenenc.RegisterUnitDataEncoders(enc.RegisterUnitDataEncoder))
	require.NoError(t, moneyenc.RegisterTxAttributeEncoders(enc.RegisterAttrEncoder))
	//require.NoError(t, enc.RegisterTxAttributeEncoders(moneyenc.RegisterTxAttributeEncoders, func(aei encoder.AttrEncID) bool { return aei.Attr == money.PayloadTypeTransfer }))

	// create tx record and tx proof pair for money transfer and serialize them into
	// CBOR array usable as predicate argument for mint and update token tx
	// Type used by wallet to serialize tx proofs into file is struct
	// type Proof struct {
	// 	_        struct{}                 `cbor:",toarray"`
	// 	TxRecord *types.TransactionRecord `json:"txRecord"`
	// 	TxProof  *types.TxProof           `json:"txProof"`
	// }
	// but we construct it manually out of raw CBOR arrays
	predicateArgs := func(t *testing.T, value uint64, refNo []byte) []byte {
		// attendee transfers to the organizer
		txPayment := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: money.DefaultSystemID,
				Type:     money.PayloadTypeTransfer,
				UnitID:   money.NewBillID(nil, []byte{8, 1, 1, 1}),
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: refNo,
				},
			},
		}
		require.NoError(t, txPayment.Payload.SetAttributes(
			money.TransferAttributes{
				NewBearer:   templates.NewP2pkh256BytesFromKey(pubKeyOrg),
				TargetValue: value,
				Counter:     1,
			}))
		require.NoError(t, txPayment.SetOwnerProof(predicates.OwnerProofer(signerAttendee, pubKeyAttendee)))

		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
		proof := testblock.CreateProof(t, txRec, signerAttendee, testblock.WithSystemIdentifier(money.DefaultSystemID))

		b, err := types.Cbor.Marshal(txRec)
		require.NoError(t, err)

		args := []types.RawCBOR{b}
		b, err = types.Cbor.Marshal(proof)
		require.NoError(t, err)

		b, err = types.Cbor.Marshal(append(args, b))
		require.NoError(t, err)

		// and wrap it into another array
		b, err = types.Cbor.Marshal([]types.RawCBOR{b})
		require.NoError(t, err)
		return b
	}

	const earlyBirdDate uint64 = 1709683200
	const regularDate uint64 = earlyBirdDate + 100000
	const earlyBirdPrice uint64 = 1000
	const regularPrice uint64 = 1500
	predArg, err := types.Cbor.Marshal([]any{earlyBirdDate, regularDate, earlyBirdPrice, regularPrice, hash.Sum256(pubKeyOrg)})
	require.NoError(t, err)

	nftTypeID := tokens.NewNonFungibleTokenTypeID(nil, []byte{7, 7, 7, 7, 7, 7, 7})
	tokenID, err := tokens.NewRandomNonFungibleTokenID(nil)
	require.NoError(t, err)

	t.Run("bearer_invariant", func(t *testing.T) {
		// evaluated when NFT is transferred to a new bearer. Checks:
		// token data is "early-bird" and date <= D1, or token data is "regular" and date <= D2
		// to get the "data", getUnit host API is called and for date "currentRound" is used
		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit([]byte{1}, &tokens.NonFungibleTokenData{Data: []byte("early-bird")}), nil
			},
			curRound:     func() uint64 { return earlyBirdDate },
			GasRemaining: 30000,
		}

		// "current transaction" for the predicate must be "transfer NFT"
		txNFTTransfer := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeTransferNFT,
				UnitID:   tokenID,
			},
		}
		require.NoError(t, txNFTTransfer.Payload.SetAttributes(
			tokens.TransferNonFungibleTokenAttributes{
				NewBearer: []byte{5, 5, 5},
				TypeID:    nftTypeID,
			}))

		obs := observability.Default(t)
		wvm, err := New(context.Background(), enc, obs)
		require.NoError(t, err)
		args := []byte{} // predicate expects no arguments
		conf := wasm.PredicateParams{Entrypoint: "bearer_invariant", Args: predArg}

		// should eval to "true"
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTTransfer, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 17171, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// hackish way to change current round past D1 so now should eval to "false"
		env.curRound = func() uint64 { return earlyBirdDate + 1 }
		start = time.Now()
		res, err = wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTTransfer, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 1, res)
		require.EqualValues(t, 4367, env.GasRemaining)
	})

	t.Run("mint_token", func(t *testing.T) {
		// org mints token (ticket) to the attendee
		txNFTMint := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeMintNFT,
				UnitID:   tokenID,
			},
			FeeProof: []byte{1, 2, 3, 4},
		}
		require.NoError(t, txNFTMint.Payload.SetAttributes(
			tokens.MintNonFungibleTokenAttributes{
				Bearer: templates.NewP2pkh256BytesFromKey(pubKeyAttendee),
				TypeID: nftTypeID,
				Data:   []byte("early-bird"),
			}))
		require.NoError(t, txNFTMint.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))

		env := &mockTxContext{
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return earlyBirdDate },
			GasRemaining: 40000,
		}
		conf := wasm.PredicateParams{Entrypoint: "mint_token", Args: predArg}

		wvm, err := New(context.Background(), enc, observability.Default(t))
		require.NoError(t, err)

		args := predicateArgs(t, earlyBirdPrice, hash.Sum256(slices.Concat([]byte{1}, nftTypeID, txNFTMint.Payload.UnitID)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTMint, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 23639, env.GasRemaining)
		require.EqualValues(t, 0x0, res)

		// set the date to future (after D1) so early-bird tickets can't be minted anymore
		env.curRound = func() uint64 { return earlyBirdDate + 1 }
		res, err = wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTMint, env)
		require.NoError(t, err)
		require.EqualValues(t, 7307, env.GasRemaining)
		require.EqualValues(t, 0x01, res)
	})

	t.Run("mint_token - error out of gas", func(t *testing.T) {
		// org mints token (ticket) to the attendee
		txNFTMint := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeMintNFT,
				UnitID:   tokenID,
			},
			FeeProof: []byte{1, 2, 3, 4},
		}
		require.NoError(t, txNFTMint.Payload.SetAttributes(
			tokens.MintNonFungibleTokenAttributes{
				Bearer: templates.NewP2pkh256BytesFromKey(pubKeyAttendee),
				Data:   []byte("early-bird"),
			}))
		require.NoError(t, txNFTMint.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))

		env := &mockTxContext{
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return earlyBirdDate },
			GasRemaining: 100,
		}
		conf := wasm.PredicateParams{Entrypoint: "mint_token", Args: predArg}

		wvm, err := New(context.Background(), enc, observability.Default(t))
		require.NoError(t, err)

		args := predicateArgs(t, earlyBirdPrice, hash.Sum256(slices.Concat([]byte{1}, nftTypeID, txNFTMint.Payload.UnitID)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTMint, env)
		t.Logf("took %s", time.Since(start))
		require.ErrorContains(t, err, "out of gas")
		require.EqualValues(t, 0, env.GasRemaining)
		require.EqualValues(t, 0x0, res)
	})

	t.Run("update_data", func(t *testing.T) {
		txNFTUpdate := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeUpdateNFT,
				UnitID:   tokenID,
			},
			OwnerProof: make([]byte, 32),
			FeeProof:   []byte{1, 2, 3, 4},
		}
		require.NoError(t, txNFTUpdate.Payload.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data: []byte("regular"),
			}))
		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit(
					[]byte{1},
					&tokens.NonFungibleTokenData{
						Name:   "Ticket 001",
						T:      42,
						Data:   []byte("early-bird"),
						TypeID: nftTypeID,
					}), nil
			},
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return regularDate },
			GasRemaining: 30000,
		}
		conf := wasm.PredicateParams{Entrypoint: "update_data", Args: predArg}

		wvm, err := New(context.Background(), enc, observability.Default(t))
		require.NoError(t, err)

		// upgrade early-bird to regular so it can be transferred after D2
		args := predicateArgs(t, regularPrice-earlyBirdPrice, hash.Sum256(slices.Concat([]byte{2}, nftTypeID, txNFTUpdate.Payload.UnitID)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTUpdate, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 8782, env.GasRemaining)
		require.EqualValues(t, 0x0, res)
	})
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)

	memDB, err := memorydb.New()
	require.NoError(t, err)

	require.NoError(t, err)
	wvm, err := New(ctx, encoder.TXSystemEncoder{}, obs, WithStorage(memDB))
	require.NoError(t, err)
	require.NotNil(t, wvm)

	require.NoError(t, wvm.Close(ctx))
}

func TestReadHeapBase(t *testing.T) {
	env := &mockTxContext{
		curRound:     func() uint64 { return 1709683000 },
		GasRemaining: 100000,
	}
	enc := encoder.TXSystemEncoder{}
	conf := wasm.PredicateParams{Entrypoint: "bearer_invariant"}
	wvm, err := New(context.Background(), enc, observability.Default(t))
	require.NoError(t, err)
	_, err = wvm.Exec(context.Background(), ticketsWasm, nil, conf, nil, env)
	require.Error(t, err)
	m, err := wvm.runtime.Instantiate(context.Background(), ticketsWasm)
	require.NoError(t, err)
	require.EqualValues(t, 8400, m.ExportedGlobal("__heap_base").Get())
	require.EqualValues(t, 8400, wvm.ctx.MemMngr.(*allocator.BumpAllocator).HeapBase())
}

func Benchmark_wazero_call_wasm_fn(b *testing.B) {
	// measure overhead of calling func from WASM
	// simple wazero setup, without any AB specific stuff.
	// addOneWasm is a WASM module which exports "add_one(x: i32) -> i32"
	b.StopTimer()
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, defaultOptions().cfg)
	defer rt.Close(ctx)

	if _, err := rt.Instantiate(ctx, envWasm); err != nil {
		b.Fatalf("instantiate env module: %v", err)
	}
	m, err := rt.Instantiate(ctx, addOneWasm)
	if err != nil {
		b.Fatal("failed to instantiate predicate code", err)
	}
	defer m.Close(ctx)

	fn := m.ExportedFunction("add_one")
	if fn == nil {
		b.Fatal("module doesn't export the add_one function")
	}

	b.StartTimer()
	var n uint64
	for i := 0; i < b.N; i++ {
		res, err := fn.Call(ctx, n)
		if err != nil {
			b.Errorf("add_one returned error: %v", err)
		}
		if n = res[0]; n != uint64(i+1) {
			b.Errorf("expected %d got %d", i, n)
		}
	}
}

type mockTxContext struct {
	getUnit      func(id types.UnitID, committed bool) (*state.Unit, error)
	payloadBytes func(txo *types.TransactionOrder) ([]byte, error)
	curRound     func() uint64
	trustBase    func() (types.RootTrustBase, error)
	GasRemaining uint64
}

func (env *mockTxContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return env.getUnit(id, committed)
}

func (env *mockTxContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return env.payloadBytes(txo)
}

func (env *mockTxContext) CurrentRound() uint64 { return env.curRound() }
func (env *mockTxContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return env.trustBase()
}
func (env *mockTxContext) GetGasRemaining() uint64 {
	return env.GasRemaining
}
func (env *mockTxContext) SpendGas(gas uint64) error {
	if gas > env.GasRemaining {
		env.GasRemaining = 0
		return fmt.Errorf("out of gas")
	}
	env.GasRemaining -= gas
	return nil
}

type mockRootTrustBase struct {
	verifyQuorumSignatures func(data []byte, signatures map[string][]byte) (error, []error)

	// instead of implementing all methods just embed the interface for now
	types.RootTrustBase
}

func (rtb *mockRootTrustBase) VerifyQuorumSignatures(data []byte, signatures map[string][]byte) (error, []error) {
	return rtb.verifyQuorumSignatures(data, signatures)
}
