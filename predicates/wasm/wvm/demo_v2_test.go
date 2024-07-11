package wvm

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	predtempl "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/predicates/wasm"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	tokenenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

//go:embed testdata/conference_tickets/v2/*.wasm
var ticketsWasmV2 embed.FS

func Test_conference_tickets_v2(t *testing.T) {
	// parameters which can be shared by all tests
	// conference organizer keys
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

	// need VerifyQuorumSignatures for verifying tx proofs of payment
	trustbase := &mockRootTrustBase{
		verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return nil, nil },
	}

	// configuration of the conference (predicate configuration)
	const earlyBirdDate uint64 = 1709683200
	const regularDate uint64 = earlyBirdDate + 100000
	const earlyBirdPrice uint64 = 1000
	const regularPrice uint64 = 1500
	predCfg, err := types.Cbor.Marshal([]any{earlyBirdDate, regularDate, earlyBirdPrice, regularPrice, hash.Sum256(pubKeyOrg)})
	require.NoError(t, err)

	// tx system unit/attribute encoder
	txsEnc := encoder.TXSystemEncoder{}
	require.NoError(t, tokenenc.RegisterTxAttributeEncoders(txsEnc.RegisterAttrEncoder))
	require.NoError(t, tokenenc.RegisterUnitDataEncoders(txsEnc.RegisterUnitDataEncoder))

	templateEng, err := predicates.Dispatcher(templates.New())
	require.NoError(t, err)

	nftTypeID := tokens.NewNonFungibleTokenTypeID(nil, []byte{7, 7, 7, 7, 7, 7, 7})
	tokenID, err := tokens.NewRandomNonFungibleTokenID(nil)
	require.NoError(t, err)

	t.Run("type_bearer", func(t *testing.T) {
		// organizer sets the bearer predicate when creating token type for tickets
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/type-bearer.wasm")
		require.NoError(t, err)

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

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "type_bearer", Args: predCfg}

		// "current transaction" for the predicate is "transfer NFT"
		txNFTTransfer := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeTransferNFT,
				UnitID:   tokenID,
			},
		}

		// token is "early-bird" and date <= D1 - should eval to "true"
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, txNFTTransfer.OwnerProof, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x4a80, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// set date past D1, should eval to false
		env.curRound = func() uint64 { return earlyBirdDate + 1 }
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTTransfer.OwnerProof, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x1fe9, env.GasRemaining)
		require.EqualValues(t, 1, res)
	})

	t.Run("type_update_data", func(t *testing.T) {
		// organizer sets the update-data predicate when creating token type for tickets
		// so it will be evaluated when update-nft tx is executed
		// expects no arguments from user, doesn't require access to the "conference configuration"
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/type-update.wasm")
		require.NoError(t, err)

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit([]byte{1}, &tokens.NonFungibleTokenData{Data: []byte("early-bird")}), nil
			},
			GasRemaining: 30000,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "type_update_data", Args: nil}

		txNFTUpdate := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeUpdateNFT,
				UnitID:   tokenID,
			},
		}
		require.NoError(t, txNFTUpdate.Payload.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("regular"),
				Counter: 2,
			}))

		// update from "early-bird" to "regular", should succeed
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, txNFTUpdate.OwnerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x4869, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// update data to "foobar", should evaluate to false
		require.NoError(t, txNFTUpdate.Payload.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("foobar"),
				Counter: 66,
			}))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTUpdate.OwnerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x2f69, env.GasRemaining)
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

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit([]byte{1}, &tokens.NonFungibleTokenData{Data: []byte("early-bird")}), nil
			},
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			payloadBytes: payloadBytes,
			GasRemaining: 50000,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "token_bearer", Args: predCfg}

		// "current transaction" for the predicate is "transfer NFT"
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

		// conference organizer transfers ticket (NFT token) to new owner.
		// as the transaction is signed by the conference organizer the predicate
		// should evaluate to true without requiring any proofs for money transfer etc
		require.NoError(t, txNFTTransfer.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, txNFTTransfer.OwnerProof, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0xa679, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// sign the transfer with some other key - p2pkh check should eval to "false" and
		// predicate carries on attempting to decode argument BLOB as proof of payment.
		// that however fails (CBOR decode) and predicate is killed with error (and tx is rejected)
		require.NoError(t, txNFTTransfer.SetOwnerProof(predicates.OwnerProofer(signerAttendee, pubKeyAttendee)))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTTransfer.OwnerProof, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.EqualError(t, err, `calling token_bearer returned error: module closed with exit_code(3134196653)`)
		assert.EqualValues(t, 0x748b, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// set the OwnerProof to BLOB containing the payment proof, token is "early-bird"
		txNFTTransfer.OwnerProof = proofOfPayment(t, signerAttendee, pubKeyOrg, earlyBirdPrice, hash.Sum256(slices.Concat([]byte{1}, txNFTTransfer.Payload.UnitID)))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTTransfer.OwnerProof, conf, txNFTTransfer, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x3ff6, env.GasRemaining)
		require.EqualValues(t, 0, res)
	})

	t.Run("token_update_data", func(t *testing.T) {
		// If the ticket was originally bought as "early-bird" and is being transferred
		// after `D1`, it must first be "upgraded" to "regular" by paying the conference
		// organizer the price difference `P2-P1`.
		predWASM, err := ticketsWasmV2.ReadFile("testdata/conference_tickets/v2/token-update.wasm")
		require.NoError(t, err)

		env := &mockTxContext{
			getUnit: func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, tokenID) {
					return nil, fmt.Errorf("unknown unit %x", id)
				}
				return state.NewUnit([]byte{1}, &tokens.NonFungibleTokenData{Data: []byte("early-bird")}), nil
			},
			trustBase:    func() (types.RootTrustBase, error) { return trustbase, nil },
			curRound:     func() uint64 { return regularDate },
			payloadBytes: payloadBytes,
			GasRemaining: 30000,
		}

		obs := observability.Default(t)
		wvm, err := New(context.Background(), txsEnc, templateEng.Execute, obs)
		require.NoError(t, err)
		conf := wasm.PredicateParams{Entrypoint: "token_update_data", Args: predCfg}

		txNFTUpdate := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: tokens.DefaultSystemID,
				Type:     tokens.PayloadTypeUpdateNFT,
				UnitID:   tokenID,
			},
		}
		require.NoError(t, txNFTUpdate.Payload.SetAttributes(
			tokens.UpdateNonFungibleTokenAttributes{
				Data:    []byte("regular"),
				Counter: 2,
			}))

		// conference organizer updates the ticket (NFT token).
		// as the transaction is signed by the conference organizer the predicate
		// should evaluate to true without requiring any proofs for money transfer etc
		require.NoError(t, txNFTUpdate.SetOwnerProof(predicates.OwnerProofer(signerOrg, pubKeyOrg)))
		start, curGas := time.Now(), env.GasRemaining
		res, err := wvm.Exec(context.Background(), predWASM, txNFTUpdate.OwnerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x58dc, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// set the OwnerProof to BLOB containing the payment proof (user upgrades the ticket)
		txNFTUpdate.OwnerProof = proofOfPayment(t, signerAttendee, pubKeyOrg, regularPrice-earlyBirdPrice, hash.Sum256(slices.Concat([]byte{2}, txNFTUpdate.Payload.UnitID)))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTUpdate.OwnerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x36ce, env.GasRemaining)
		require.EqualValues(t, 0, res)

		// user attempts to upgrade the ticket but the sum (amount of money transferred) in the payment proof is wrong
		txNFTUpdate.OwnerProof = proofOfPayment(t, signerAttendee, pubKeyOrg, regularPrice-earlyBirdPrice-1, hash.Sum256(slices.Concat([]byte{2}, txNFTUpdate.Payload.UnitID)))
		start, curGas = time.Now(), env.GasRemaining
		res, err = wvm.Exec(context.Background(), predWASM, txNFTUpdate.OwnerProof, conf, txNFTUpdate, env)
		t.Logf("took %s, spent %d gas", time.Since(start), curGas-env.GasRemaining)
		require.NoError(t, err)
		assert.EqualValues(t, 0x14c0, env.GasRemaining)
		require.EqualValues(t, 0x701, res, "expected code `7` = transferred amount doesn't equal to `P2 - P1`")
	})
}

// create tx record and tx proof pair for money transfer and serialize them into
// CBOR array usable as predicate argument for mint and update token tx
// Type used by wallet to serialize tx proofs into file is struct
//
//	type Proof struct {
//		_        struct{}                 `cbor:",toarray"`
//		TxRecord *types.TransactionRecord `json:"txRecord"`
//		TxProof  *types.TxProof           `json:"txProof"`
//	}
//
// but we construct it manually out of raw CBOR arrays
func proofOfPayment(t *testing.T, signer abcrypto.Signer, receiverPK []byte, value uint64, refNo []byte) []byte {
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
			NewBearer:   predtempl.NewP2pkh256BytesFromKey(receiverPK),
			TargetValue: value,
			Counter:     1,
		}))
	require.NoError(t, txPayment.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))

	txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
	proof := testblock.CreateProof(t, txRec, signer, testblock.WithSystemIdentifier(money.DefaultSystemID))

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

/*
PayloadBytes returns txo payload bytes (bytes signed by bearer).
This hack is needed until AB-1012 gets resolved.
*/
func payloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	attr, err := attrType(txo.PayloadType())
	if err != nil {
		return nil, err
	}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, err
	}
	buf, err := txo.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func attrType(payloadType string) (types.SigBytesProvider, error) {
	switch payloadType {
	case tokens.PayloadTypeCreateNFTType:
		return &tokens.CreateNonFungibleTokenTypeAttributes{}, nil
	case tokens.PayloadTypeMintNFT:
		return &tokens.MintNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeTransferNFT:
		return &tokens.TransferNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeUpdateNFT:
		return &tokens.UpdateNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeCreateFungibleTokenType:
		return &tokens.CreateFungibleTokenTypeAttributes{}, nil
	case tokens.PayloadTypeMintFungibleToken:
		return &tokens.MintFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeTransferFungibleToken:
		return &tokens.TransferFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeSplitFungibleToken:
		return &tokens.SplitFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeBurnFungibleToken:
		return &tokens.BurnFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeJoinFungibleToken:
		return &tokens.JoinFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeLockToken:
		return &tokens.LockTokenAttributes{}, nil
	case tokens.PayloadTypeUnlockToken:
		return &tokens.UnlockTokenAttributes{}, nil
	}
	return nil, fmt.Errorf("unknown payload type %q", payloadType)
}
