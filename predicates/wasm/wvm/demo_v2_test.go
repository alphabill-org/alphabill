package wvm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	predtempl "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	tokenenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

//go:embed testdata/conference_tickets/v2/*.wasm
var ticketsWasmV2 embed.FS

func Test_conference_tickets_v2(t *testing.T) {
	checkSpentGas := func(t *testing.T, expected, actual uint64) {
		t.Helper()
		assert.Equal(t, expected, actual, "amount of gas spent, diff %d", max(expected, actual)-min(expected, actual))
	}
	// parameters which can be shared by all tests
	// conference organizer keys
	signerOrg, organizerPKH := signerAndPKH(t)
	// customer buying conference ticket
	signerAttendee, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	// need VerifyQuorumSignatures for verifying tx proofs of payment
	trustbase := &mockRootTrustBase{
		verifyQuorumSignatures: func(data []byte, signatures map[string]hex.Bytes) (error, []error) { return nil, nil },
	}
	orchestration := mockOrchestration{
		trustBase: func(epoch uint64) (types.RootTrustBase, error) { return trustbase, nil },
	}

	// configuration of the conference (predicate configuration)
	const earlyBirdDate uint64 = 1709683200
	const regularDate uint64 = earlyBirdDate + 100000
	const earlyBirdPrice uint64 = 1000
	const regularPrice uint64 = 1500
	predCfg, err := types.Cbor.Marshal([]any{[]any{earlyBirdDate, regularDate, earlyBirdPrice, regularPrice}, organizerPKH})
	require.NoError(t, err)

	// tx system unit/attribute encoder
	txsEnc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(tokens.DefaultPartitionID, txsEnc.RegisterAttrEncoder))
	require.NoError(t, tokenenc.RegisterUnitDataEncoders(txsEnc.RegisterUnitDataEncoder))

	tmpPred, err := templates.New(observability.Default(t))
	require.NoError(t, err)
	templateEng, err := predicates.Dispatcher(tmpPred)
	require.NoError(t, err)

	nftTypeID := tokenid.NewNonFungibleTokenTypeID(t)
	tokenID := tokenid.NewNonFungibleTokenID(t)

	t.Run("type_bearer", func(t *testing.T) {
		// organizer sets the bearer predicate when creating token type for tickets
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/type-bearer.wasm")
		require.NoError(t, err)

		// "current transaction" for the predicate is "transfer NFT"
		txNFTTransfer := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeTransferNFT,
				UnitID:      tokenID,
			},
		}

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit(&tokens.NonFungibleTokenData{Data: []byte("early-bird"), OwnerPredicate: []byte{1}}), nil
			},
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: earlyBirdDate},
				}
			},
			GasRemaining: 30000,
			exArgument:   txNFTTransfer.AuthProofSigBytes,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, orchestration, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "type_bearer", Args: predCfg}

		// token is "early-bird" and date <= D1 - should eval to "true"
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, nil, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 7633, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)

		// set date past D1, should eval to false
		env.committedUC = func() *types.UnicityCertificate {
			return &types.UnicityCertificate{
				UnicitySeal: &types.UnicitySeal{Timestamp: earlyBirdDate + 1},
			}
		}
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, nil, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 7626, curGas-env.GasRemaining)
		require.EqualValues(t, 1, res)
	})

	t.Run("type_update_data", func(t *testing.T) {
		t.SkipNow() // TODO update type-update.wasm (field "Locked" was removed)

		// organizer sets the update-data predicate when creating token type for tickets
		// so it will be evaluated when update-nft tx is executed
		// expects no arguments from user, doesn't require access to the "conference configuration"
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/type-update.wasm")
		require.NoError(t, err)

		txNFTUpdate := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeUpdateNFT,
				UnitID:      tokenID,
			},
		}
		require.NoError(t, txNFTUpdate.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("regular"),
				Counter: 2,
			}))

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit(&tokens.NonFungibleTokenData{Data: []byte("early-bird"), OwnerPredicate: []byte{1}}), nil
			},
			GasRemaining: 30000,
			exArgument:   txNFTUpdate.AuthProofSigBytes,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, orchestration, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "type_update_data", Args: nil}

		// update from "early-bird" to "regular", should succeed
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, nil, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 3361, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)

		// update data to "foobar", should evaluate to false
		require.NoError(t, txNFTUpdate.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("foobar"),
				Counter: 66,
			}))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, nil, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5191, curGas-env.GasRemaining)
		require.EqualValues(t, 0x201, res, `expected error code 2: new value of the token data is not "regular"`)
	})

	t.Run("token_bearer", func(t *testing.T) {
		// predicate is used as "bearer predicate" by the conference organizer when
		// minting an token (ticket) for "open-market" (ie not to concrete receiver).
		// When organizer receives (ie off-line) payment he can transfer the token to
		// payer's P2PKH by signing the transfer with his own key.
		// Alternatively someone can transfer required amount of money to the organizer
		// and use proof of that transfer as a OwnerProof for transferring token
		// to their own PubKey.
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/token-bearer.wasm")
		require.NoError(t, err)

		// "current transaction" for the predicate is "transfer NFT"
		txNFTTransfer := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeTransferNFT,
				UnitID:      tokenID,
			},
		}
		require.NoError(t, txNFTTransfer.SetAttributes(
			tokens.TransferNonFungibleTokenAttributes{
				NewOwnerPredicate: []byte{5, 5, 5},
				TypeID:            nftTypeID,
			}))

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit(&tokens.NonFungibleTokenData{Data: []byte("early-bird"), OwnerPredicate: []byte{1}}), nil
			},
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			GasRemaining: 50000,
			exArgument:   txNFTTransfer.AuthProofSigBytes,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, orchestration, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "token_bearer", Args: predCfg}

		// conference organizer transfers ticket (NFT token) to new owner.
		// as the transaction is signed by the conference organizer the predicate
		// should evaluate to true without requiring any proofs for money transfer etc
		ownerProofOrg := testsig.NewAuthProofSignature(t, txNFTTransfer, signerOrg)
		require.NoError(t, txNFTTransfer.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: ownerProofOrg,
		}))

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, ownerProofOrg, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5334, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)

		// sign the transfer with some other key - p2pkh check should eval to "false" and predicate
		// returns "false" without any further work (ie checking for money transfer).
		ownerProofAttendee := testsig.NewAuthProofSignature(t, txNFTTransfer, signerAttendee)
		require.NoError(t, txNFTTransfer.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: ownerProofAttendee,
		}))

		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, ownerProofAttendee, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5320, curGas-env.GasRemaining)
		require.EqualValues(t, 0x801, res)

		// set the OwnerProof to BLOB containing the payment proof, token is "early-bird"
		// attempt to eval p2pkh with OwnerProof as argument should fail (returns error) and
		// predicate should carry on attempting to decode argument BLOB as proof of payment.
		refNo := sha256.Sum256(slices.Concat([]byte{1}, txNFTTransfer.UnitID))
		ownerProofAttendee = proofOfPayment(t, signerAttendee, organizerPKH, earlyBirdPrice, refNo[:])
		require.NoError(t, txNFTTransfer.SetAuthProof(&tokens.TransferNonFungibleTokenAuthProof{
			OwnerProof: ownerProofAttendee,
		}))

		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, ownerProofAttendee, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 10295, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)
	})

	t.Run("token_update_data", func(t *testing.T) {
		// If the ticket was originally bought as "early-bird" and is being transferred
		// after `D1`, it must first be "upgraded" to "regular" by paying the conference
		// organizer the price difference `P2-P1`.
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/token-update.wasm")
		require.NoError(t, err)

		txNFTUpdate := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeUpdateNFT,
				UnitID:      tokenID,
			},
		}
		require.NoError(t, txNFTUpdate.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("regular"),
				Counter: 2,
			}))

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit(&tokens.NonFungibleTokenData{Data: []byte("early-bird"), OwnerPredicate: []byte{1}}), nil
			},
			trustBase: func() (types.RootTrustBase, error) { return trustbase, nil },
			committedUC: func() *types.UnicityCertificate {
				return &types.UnicityCertificate{
					UnicitySeal: &types.UnicitySeal{Timestamp: regularDate},
				}
			},
			GasRemaining: 30000,
			exArgument:   txNFTUpdate.AuthProofSigBytes,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, orchestration, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "token_update_data", Args: predCfg}

		// conference organizer updates the ticket (NFT token).
		// as the transaction is signed by the conference organizer the predicate
		// should evaluate to true without requiring any proofs for money transfer etc
		ownerProofOrg := testsig.NewAuthProofSignature(t, txNFTUpdate, signerOrg)
		require.NoError(t, txNFTUpdate.SetAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{
			TokenDataUpdateProof: ownerProofOrg,
		}))

		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, ownerProofOrg, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 5212, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)

		// when user "upgrades the ticket" the owner proof must be proof of money transfer to the
		// conference organizer with reference number = sha256(2, unitID)
		refNo := sha256.Sum256(slices.Concat([]byte{2}, txNFTUpdate.UnitID))
		ownerProof := proofOfPayment(t, signerAttendee, organizerPKH, regularPrice-earlyBirdPrice, refNo[:])
		require.NoError(t, txNFTUpdate.SetAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{
			TokenDataUpdateProof: ownerProof,
		}))

		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, ownerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 6762, curGas-env.GasRemaining)
		require.EqualValues(t, 0, res)

		// user attempts to upgrade the ticket but the sum (amount of money transferred) in the payment proof is wrong
		ownerProof = proofOfPayment(t, signerAttendee, organizerPKH, regularPrice-earlyBirdPrice-1, refNo[:])
		require.NoError(t, txNFTUpdate.SetAuthProof(&tokens.UpdateNonFungibleTokenAuthProof{
			TokenDataUpdateProof: ownerProof,
		}))

		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, ownerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		checkSpentGas(t, 6762, curGas-env.GasRemaining)
		require.EqualValues(t, 0x701, res, "expected code `7` = transferred amount doesn't equal to `P2 - P1`")
	})
}

// create tx record and tx proof pair for money transfer and serialize them into
// CBOR array usable as predicate argument for mint and update token tx
func proofOfPayment(t *testing.T, signer abcrypto.Signer, receiverPKH []byte, value uint64, refNo []byte) []byte {
	// attendee transfers to the organizer
	txPayment := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID: money.DefaultPartitionID,
			Type:        money.TransactionTypeTransfer,
			UnitID:      moneyid.NewBillID(t),
			ClientMetadata: &types.ClientMetadata{
				ReferenceNumber: refNo,
			},
		},
	}
	require.NoError(t, txPayment.SetAttributes(
		money.TransferAttributes{
			NewOwnerPredicate: predtempl.NewP2pkh256BytesFromKeyHash(receiverPKH),
			TargetValue:       value,
			Counter:           1,
		}))
	require.NoError(t, txPayment.SetAuthProof(
		&tokens.TransferNonFungibleTokenAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, txPayment, signer)}),
	)

	txBytes, err := txPayment.MarshalCBOR()
	require.NoError(t, err)
	txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
	txRecProof := testblock.CreateTxRecordProof(t, txRec, signer, testblock.WithPartitionID(money.DefaultPartitionID))

	b, err := types.Cbor.Marshal([]*types.TxRecordProof{txRecProof})
	require.NoError(t, err)
	return b
}
