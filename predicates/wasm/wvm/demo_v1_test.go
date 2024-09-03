package wvm

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"slices"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	moneyenc "github.com/alphabill-org/alphabill/txsystem/money/encoder"
	tokenenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/conference_tickets/v1/conf_tickets.wasm
var ticketsWasm []byte

func Test_conference_tickets_v1(t *testing.T) {
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
	// need VerifyQuorumSignatures for verifying tx proofs
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
				NewOwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKeyOrg),
				TargetValue:       value,
				Counter:           1,
			}))
		require.NoError(t, txPayment.SetAuthProof(
			money.TransferAuthProof{OwnerProof: testsig.NewOwnerProof(t, txPayment, signerAttendee)}),
		)
		//require.NoError(t, txPayment.SetOwnerProof(predicates.OwnerProofer(signerAttendee, pubKeyAttendee)))

		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
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
				NewOwnerPredicate: []byte{5, 5, 5},
				TypeID:            nftTypeID,
			}))

		obs := observability.Default(t)
		wvm, err := New(context.Background(), enc, nil, obs)
		require.NoError(t, err)
		args := []byte{} // predicate expects no arguments
		conf := wasm.PredicateParams{Entrypoint: "bearer_invariant", Args: predArg}

		// should eval to "true"
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTTransfer, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 18307, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// hackish way to change current round past D1 so now should eval to "false"
		env.curRound = func() uint64 { return earlyBirdDate + 1 }
		start = time.Now()
		res, err = wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTTransfer, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 1, res)
		require.EqualValues(t, 6639, env.GasRemaining)
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
				OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKeyAttendee),
				TypeID:         nftTypeID,
				Data:           []byte("early-bird"),
			}))
		require.NoError(t, txNFTMint.SetAuthProof(
			tokens.MintNonFungibleTokenAuthProof{TokenMintingPredicateSignature: testsig.NewOwnerProof(t, txNFTMint, signerOrg)}),
		)
		//require.NoError(t, txNFTMint.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))

		env := &mockTxContext{
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return earlyBirdDate },
			GasRemaining: 40000,
		}
		conf := wasm.PredicateParams{Entrypoint: "mint_token", Args: predArg}

		wvm, err := New(context.Background(), enc, nil, observability.Default(t))
		require.NoError(t, err)

		args := predicateArgs(t, earlyBirdPrice, hash.Sum256(slices.Concat([]byte{1}, nftTypeID, txNFTMint.Payload.UnitID)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTMint, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 24077, env.GasRemaining)
		require.EqualValues(t, 0x0, res)

		// set the date to future (after D1) so early-bird tickets can't be minted anymore
		env.curRound = func() uint64 { return earlyBirdDate + 1 }
		res, err = wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTMint, env)
		require.NoError(t, err)
		require.EqualValues(t, 8183, env.GasRemaining)
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
				OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKeyAttendee),
				Data:           []byte("early-bird"),
			}))
		require.NoError(t, txNFTMint.SetAuthProof(
			tokens.MintNonFungibleTokenAuthProof{TokenMintingPredicateSignature: testsig.NewOwnerProof(t, txNFTMint, signerOrg)}),
		)
		//require.NoError(t, txNFTMint.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))

		env := &mockTxContext{
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return earlyBirdDate },
			GasRemaining: 100,
		}
		conf := wasm.PredicateParams{Entrypoint: "mint_token", Args: predArg}

		wvm, err := New(context.Background(), enc, nil, observability.Default(t))
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
			AuthProof: make([]byte, 32),
			FeeProof:  []byte{1, 2, 3, 4},
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

		wvm, err := New(context.Background(), enc, nil, observability.Default(t))
		require.NoError(t, err)

		// upgrade early-bird to regular so it can be transferred after D2
		args := predicateArgs(t, regularPrice-earlyBirdPrice, hash.Sum256(slices.Concat([]byte{2}, nftTypeID, txNFTUpdate.Payload.UnitID)))
		start := time.Now()
		res, err := wvm.Exec(context.Background(), ticketsWasm, args, conf, txNFTUpdate, env)
		t.Logf("took %s", time.Since(start))
		require.NoError(t, err)
		require.EqualValues(t, 9389, env.GasRemaining)
		require.EqualValues(t, 0x0, res)
	})
}
