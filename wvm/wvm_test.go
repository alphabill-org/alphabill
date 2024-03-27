package wvm

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/encoder"
	moneyenc "github.com/alphabill-org/alphabill/wvm/encoder/money"
	tokenenc "github.com/alphabill-org/alphabill/wvm/encoder/token"
)

//go:embed testdata/add_one/target/wasm32-unknown-unknown/release/add_one.wasm
var addOneWasm []byte

//go:embed testdata/missingHostAPI/target/wasm32-unknown-unknown/release/invalid_api.wasm
var invalidAPIWasm []byte

//go:embed testdata/p2pkh_v1/p2pkh.wasm
var p2pkhV1Wasm []byte

//go:embed testdata/tickets.wasm
var ticketsWasm []byte

func Test_conference_tickets(t *testing.T) {

	// conference organiser
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
	trustbase := map[string]abcrypto.Verifier{"test": verifierAttendee, "attendee": verifierAttendee, "org": verifierOrg}

	enc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(enc.RegisterAttrEncoder))
	require.NoError(t, tokenenc.RegisterUnitDataEncoders(enc.RegisterUnitDataEncoder))
	require.NoError(t, moneyenc.RegisterTxAttributeEncoders(enc.RegisterAttrEncoder))
	//require.NoError(t, enc.RegisterTxAttributeEncoders(moneyenc.RegisterTxAttributeEncoders, func(aei encoder.AttrEncID) bool { return aei.Attr == money.PayloadTypeTransfer }))

	// create tx record and tx proof pair for money transfer and serialize them into
	// CBOR array usable as predicate argument for mint and update token tx
	predicateArgs := func(t *testing.T, value uint64, nonce []byte) []byte {
		// attendee transfers to the organiser
		txPayment := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: money.DefaultSystemIdentifier,
				Type:     money.PayloadTypeTransfer,
				UnitID:   money.NewBillID(nil, []byte{8, 1, 1, 1}),
			},
		}
		require.NoError(t, txPayment.Payload.SetAttributes(
			money.TransferAttributes{
				NewBearer:   templates.NewP2pkh256BytesFromKey(pubKeyOrg),
				TargetValue: value,
				Backlink:    []byte{3, 3},
				//Nonce:       nonce, // AB-1509
			}))
		require.NoError(t, txPayment.SetOwnerProof(predicates.OwnerProofer(signerAttendee, pubKeyAttendee)))

		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
		proof := testblock.CreateProof(t, txRec, signerAttendee, testblock.WithSystemIdentifier(money.DefaultSystemIdentifier))

		b, err := cbor.Marshal(txRec)
		require.NoError(t, err)

		args := []types.RawCBOR{b}
		b, err = cbor.Marshal(proof)
		require.NoError(t, err)
		args = append(args, b)

		b, err = cbor.Marshal(args)
		require.NoError(t, err)
		return b
	}

	/*
		params hardcoded to the predicates:
		const D1: u64 = 1709683200;
		const D2: u64 = D1+ 100000;
		const P1: u64 = 1000;
		const P2: u64 = 1500;
	*/

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
			curRound: func() uint64 { return 1709683000 },
		}

		// "current transaction" for the predicate must be "transfer NFT"
		txNFTTransfer := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemIdentifier,
				Type:     tokens.PayloadTypeTransferNFT,
				UnitID:   tokenID,
			},
		}
		require.NoError(t, txNFTTransfer.Payload.SetAttributes(
			tokens.TransferNonFungibleTokenAttributes{
				NewBearer: []byte{5, 5, 5},
				NFTTypeID: nftTypeID,
			}))

		obs := observability.Default(t)
		wvm, err := New(context.Background(), enc, env, obs)
		require.NoError(t, err)
		args := []byte{} // predicate expects no arguments

		// should eval to "true"
		start := time.Now()
		res, err := wvm.Exec(context.Background(), "bearer_invariant", ticketsWasm, args, txNFTTransfer)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 0, res)

		// hakich way to change current round so now should eval to "false"
		env.curRound = func() uint64 { return 1709684000 }
		start = time.Now()
		res, err = wvm.Exec(context.Background(), "bearer_invariant", ticketsWasm, args, txNFTTransfer)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 1, res)
	})

	t.Run("mint_token", func(t *testing.T) {
		// org mints token (ticket) to the attendee
		txNFTMint := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemIdentifier,
				Type:     tokens.PayloadTypeMintNFT,
				UnitID:   tokenID,
			},
			FeeProof: []byte{1, 2, 3, 4},
		}
		require.NoError(t, txNFTMint.Payload.SetAttributes(
			tokens.MintNonFungibleTokenAttributes{
				Bearer:    templates.NewP2pkh256BytesFromKey(pubKeyAttendee),
				NFTTypeID: nftTypeID,
				Data:      []byte("early-bird"),
			}))
		require.NoError(t, txNFTMint.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))

		env := &mockTxContext{
			trustBase: func() (map[string]abcrypto.Verifier, error) { return trustbase, nil },
			curRound:  func() uint64 { return 1709683000 },
		}

		wvm, err := New(context.Background(), enc, env, observability.Default(t))
		require.NoError(t, err)

		args := predicateArgs(t, 100, hash.Sum256(append(append([]byte{1}, nftTypeID...), txNFTMint.Payload.UnitID...)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), "mint_token", ticketsWasm, args, txNFTMint)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		// verifyTxProof: nonce doesn't match - we currently do not have nonce in the transfer money attributes!
		require.EqualValues(t, 0x0208, res)
	})

	t.Run("update_data", func(t *testing.T) {
		txNFTUpdate := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemIdentifier,
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
						Name:    "Ticket 001",
						T:       42,
						Data:    []byte("early-bird"),
						NftType: nftTypeID,
					}), nil
			},
			trustBase: func() (map[string]abcrypto.Verifier, error) { return trustbase, nil },
			curRound:  func() uint64 { return 1709683000 },
		}

		wvm, err := New(context.Background(), enc, env, observability.Default(t))
		require.NoError(t, err)

		args := predicateArgs(t, 100, hash.Sum256(append(append([]byte{1}, nftTypeID...), txNFTUpdate.Payload.UnitID...)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), "update_data", ticketsWasm, args, txNFTUpdate)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		// verifyTxProof: nonce doesn't match - we currently do not have nonce in the transfer money attributes!
		require.EqualValues(t, 0x0208, res)
	})
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	obs := observability.Default(t)

	memDB, err := memorydb.New()
	require.NoError(t, err)
	env := &mockTxContext{}

	require.NoError(t, err)
	wvm, err := New(ctx, encoder.TXSystemEncoder{}, env, obs, WithStorage(memDB))
	require.NoError(t, err)
	require.NotNil(t, wvm)

	require.NoError(t, wvm.Close(ctx))
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
