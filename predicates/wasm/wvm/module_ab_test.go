package wvm

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/bumpallocator"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

func Test_txSignedByPKH(t *testing.T) {
	buildContext := func(t *testing.T) (context.Context, *vmContext, *mockApiMod) {
		txsEnc := encoder.TXSystemEncoder{}

		obs := observability.Default(t)
		vm := &vmContext{
			curPrg: &evalContext{
				vars:   map[uint32]any{},
				varIdx: handle_max_reserved,
			},
			encoder: txsEnc,
			memMngr: bumpallocator.New(0, maxMem(10000)),
			log:     obs.Logger(),
		}
		mem := &mockMemory{
			size: func() uint32 { return 10000 },
		}
		mod := &mockApiMod{memory: func() api.Memory { return mem }}
		return context.WithValue(context.Background(), runtimeContextKey, vm), vm, mod
	}

	pkh := []byte{41, 66, 80, 41, 66, 80, 41, 66, 80}
	txo := types.TransactionOrder{Version: 1, Payload: types.Payload{PartitionID: tokens.DefaultPartitionID, Type: tokens.TransactionTypeTransferNFT}}

	t.Run("invalid proof handle", func(t *testing.T) {
		// the proof is sent by tx system and is set by the WASM engine as
		// handle_current_args so it must exist but could be nil?
		ctx, vm, mod := buildContext(t)
		vm.curPrg.vars[handle_current_tx_order] = &txo
		handle_pkh := uint64(vm.curPrg.addVar(pkh))

		// proof handle (handle_current_args) has no value registered
		stack := []uint64{handle_current_tx_order, handle_pkh, handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 5, stack[0])

		// handle refers to nil
		stack = []uint64{handle_current_tx_order, handle_pkh, uint64(vm.curPrg.addVar(nil))}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 7, stack[0])
	})

	t.Run("invalid PKH handle", func(t *testing.T) {
		ctx, vm, mod := buildContext(t)
		vm.curPrg.vars[handle_current_tx_order] = &txo
		vm.curPrg.vars[handle_current_args] = []byte("owner proof")

		// handle refers to nonexisting var
		stack := []uint64{handle_current_tx_order, uint64(vm.curPrg.varIdx), handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 4, stack[0])

		// handle refers to nil
		stack = []uint64{handle_current_tx_order, uint64(vm.curPrg.addVar(nil)), handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 6, stack[0])
	})

	t.Run("invalid txo handle", func(t *testing.T) {
		ctx, _, mod := buildContext(t)
		stack := []uint64{handle_current_tx_order, 0, handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 3, stack[0])
	})

	t.Run("evaluating p2pkh returns error", func(t *testing.T) {
		expErr := errors.New("predicate eval failure")
		ctx, vm, mod := buildContext(t)
		vm.curPrg.vars[handle_current_tx_order] = &txo
		vm.curPrg.vars[handle_current_args] = []byte("owner proof")
		predicateExecuted := false
		vm.engines = func(context.Context, types.PredicateBytes, []byte, *types.TransactionOrder, predicates.TxContext) (bool, error) {
			predicateExecuted = true
			return true, expErr
		}
		stack := []uint64{handle_current_tx_order, uint64(vm.curPrg.addVar(pkh)), handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 2, stack[0])
		require.True(t, predicateExecuted, "call predicate engine")
	})

	t.Run("evaluating p2pkh returns false", func(t *testing.T) {
		ctx, vm, mod := buildContext(t)
		vm.curPrg.vars[handle_current_tx_order] = &txo
		vm.curPrg.vars[handle_current_args] = []byte("owner proof")
		predicateExecuted := false
		vm.engines = func(context.Context, types.PredicateBytes, []byte, *types.TransactionOrder, predicates.TxContext) (bool, error) {
			predicateExecuted = true
			return false, nil
		}
		stack := []uint64{handle_current_tx_order, uint64(vm.curPrg.addVar(pkh)), handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 1, stack[0])
		require.True(t, predicateExecuted, "call predicate engine")
	})

	t.Run("evaluating p2pkh returns true", func(t *testing.T) {
		ctx, vm, mod := buildContext(t)
		txOrder := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				Type:        tokens.TransactionTypeTransferNFT,
				PartitionID: tokens.DefaultPartitionID,
			},
		}
		ownerProof := []byte{9, 8, 0}

		authProofSigBytes, err := txOrder.AuthProofSigBytes()
		require.NoError(t, err)

		vm.curPrg.vars[handle_current_tx_order] = txOrder
		vm.curPrg.vars[handle_current_args] = ownerProof
		predicateExecuted := false
		vm.engines = func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
			predicateExecuted = true

			require.Equal(t, txOrder, txo)

			sigBytes, err := env.ExtraArgument()
			require.NoError(t, err)
			require.Equal(t, authProofSigBytes, sigBytes)

			require.EqualValues(t, ownerProof, args)
			h, err := templates.ExtractPubKeyHashFromP2pkhPredicate(predicate)
			require.NoError(t, err)
			require.Equal(t, pkh, h)
			return true, nil
		}

		stack := []uint64{handle_current_tx_order, uint64(vm.curPrg.addVar(pkh)), handle_current_args}
		txSignedByPKH(ctx, mod, stack)
		require.EqualValues(t, 0, stack[0])
		require.True(t, predicateExecuted, "call predicate engine")
	})
}

func Test_amountTransferredSum(t *testing.T) {
	// trustbase which "successfully" verifies all tx proofs (ie says they're valid)
	trustBaseOK := &mockRootTrustBase{
		// need VerifyQuorumSignatures for verifying tx proofs
		verifyQuorumSignatures: func(data []byte, signatures map[string]hex.Bytes) (error, []error) { return nil, nil },
	}
	tbLoader := func(epoch uint64) (types.RootTrustBase, error) { return trustBaseOK, nil }

	tbSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	// public key hashes
	pkhA := []byte{3, 8, 0, 1, 2, 4, 5}
	pkhB := []byte{3, 8, 0, 1, 2, 4, 0}
	// create mix of tx types
	// add an invalid proof record - just tx record, proof is missing
	proofs := []*types.TxRecordProof{
		{
			TxRecord: &types.TransactionRecord{
				Version:          1,
				TransactionOrder: testtransaction.NewTransactionOrderBytes(t),
				ServerMetadata:   &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful},
			},
			TxProof: nil,
		},
	}
	// valid money transfer
	txPayment := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID: money.DefaultPartitionID,
			Type:        money.TransactionTypeTransfer,
		},
	}
	err = txPayment.SetAttributes(money.TransferAttributes{
		NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkhA),
		TargetValue:       100,
	})
	require.NoError(t, err)

	txPaymentBytes, err := txPayment.MarshalCBOR()
	require.NoError(t, err)
	txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txPaymentBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
	txRecProof := testblock.CreateTxRecordProof(t, txRec, tbSigner, testblock.WithPartitionID(money.DefaultPartitionID))
	proofs = append(proofs, txRecProof)

	// money transfer by split tx
	txPayment = &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			PartitionID: money.DefaultPartitionID,
			Type:        money.TransactionTypeSplit,
		},
	}
	err = txPayment.SetAttributes(money.SplitAttributes{
		TargetUnits: []*money.TargetUnit{
			{Amount: 10, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkhA)},
			{Amount: 50, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkhB)},
			{Amount: 90, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkhA)},
		},
	})
	require.NoError(t, err)

	txPaymentBytes, err = txPayment.MarshalCBOR()
	require.NoError(t, err)
	txRec = &types.TransactionRecord{Version: 1, TransactionOrder: txPaymentBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
	txRecProof = testblock.CreateTxRecordProof(t, txRec, tbSigner, testblock.WithPartitionID(money.DefaultPartitionID))
	proofs = append(proofs, txRecProof)

	// because of invalid proof record we expect error but pkhA should receive
	// total of 200 (transfer=100 + split=10+90)
	sum, err := amountTransferredSum(proofs, pkhA, nil, tbLoader)
	require.EqualError(t, err, `record[0]: invalid input: transaction proof is nil`)
	require.EqualValues(t, 200, sum)
	// pkhB should get 50 from split
	sum, err = amountTransferredSum(proofs, pkhB, nil, tbLoader)
	require.EqualError(t, err, `record[0]: invalid input: transaction proof is nil`)
	require.EqualValues(t, 50, sum)
	// nil as pkh
	sum, err = amountTransferredSum(proofs[1:], nil, nil, tbLoader)
	require.NoError(t, err)
	require.Zero(t, sum)

	// set the epoch in the UnicitySeal of the last tx (split)
	uc, err := txRecProof.TxProof.GetUC()
	require.NoError(t, err)
	uc.UnicitySeal.Epoch = 160921087
	txRecProof.TxProof.UnicityCertificate, err = uc.MarshalCBOR()
	require.NoError(t, err)
	// loader which finds trust base only for the epoch in the split tx proof
	tbl := func(epoch uint64) (types.RootTrustBase, error) {
		if epoch == uc.UnicitySeal.Epoch {
			return trustBaseOK, nil
		}
		return nil, errors.New("nope")
	}
	sum, err = amountTransferredSum(proofs, pkhA, nil, tbl)
	require.EqualError(t, err, "record[0]: invalid input: transaction proof is nil\nrecord[1]: verification of transaction: acquiring trust base: nope")
	require.EqualValues(t, 100, sum)
}

func Test_transferredSum(t *testing.T) {
	// trustbase which "successfully" verifies all tx proofs (ie says they're valid)
	trustBaseOK := &mockRootTrustBase{
		// need VerifyQuorumSignatures for verifying tx proofs
		verifyQuorumSignatures: func(data []byte, signatures map[string]hex.Bytes) (error, []error) { return nil, nil },
	}
	tbLoader := func(epoch uint64) (types.RootTrustBase, error) { return trustBaseOK, nil }

	t.Run("invalid input, required argument is nil", func(t *testing.T) {
		receiverPKH := make([]byte, 32)
		txo := types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: 1,
				Type:        money.TransactionTypeTransfer,
			},
		}
		require.NoError(t, txo.SetAttributes(money.TransferAttributes{NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(receiverPKH)}))
		b, err := txo.MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{Version: 1, TransactionOrder: b, ServerMetadata: &types.ServerMetadata{}}
		txRecProof := &types.TxRecordProof{TxRecord: txRec, TxProof: &types.TxProof{Version: 1}}

		tbl := func(epoch uint64) (types.RootTrustBase, error) { return nil, errors.New("unexpected call") }
		sum, err := transferredSum(txRecProof, receiverPKH, nil, tbl)
		require.Zero(t, sum)
		require.EqualError(t, err, `verification of transaction: reading UC of the tx proof: unicity certificate is nil`)

		sum, err = transferredSum(nil, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: transaction record proof is nil`)

		invalidTxRecProof := &types.TxRecordProof{TxRecord: nil, TxProof: &types.TxProof{Version: 1}}
		sum, err = transferredSum(invalidTxRecProof, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: transaction record is nil`)

		invalidTxRecProof = &types.TxRecordProof{TxRecord: &types.TransactionRecord{Version: 1, TransactionOrder: nil}, TxProof: &types.TxProof{Version: 1}}
		sum, err = transferredSum(invalidTxRecProof, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: transaction order is nil`)

		invalidTxRecProof = &types.TxRecordProof{TxRecord: &types.TransactionRecord{Version: 1, TransactionOrder: testtransaction.NewTransactionOrderBytes(t), ServerMetadata: nil}, TxProof: &types.TxProof{Version: 1}}
		sum, err = transferredSum(invalidTxRecProof, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: server metadata is nil`)

		invalidTxRecProof = &types.TxRecordProof{TxRecord: &types.TransactionRecord{Version: 1, TransactionOrder: testtransaction.NewTransactionOrderBytes(t), ServerMetadata: &types.ServerMetadata{}}, TxProof: nil}
		sum, err = transferredSum(invalidTxRecProof, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: transaction proof is nil`)
	})

	t.Run("tx for non-money txsystem", func(t *testing.T) {
		// money partition ID is 1, create tx for some other txs
		txPaymentBytes, err := (&types.TransactionOrder{Version: 1, Payload: types.Payload{PartitionID: 2}}).MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txPaymentBytes, ServerMetadata: &types.ServerMetadata{}}
		txRecProof := &types.TxRecordProof{TxRecord: txRec, TxProof: &types.TxProof{Version: 1}}
		sum, err := transferredSum(txRecProof, nil, nil, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `expected partition id 1 got 2`)
	})

	t.Run("ref number mismatch", func(t *testing.T) {
		// if ref-no parameter is provided it must match (nil ref-no means "do not care")
		tx := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: money.DefaultPartitionID,
				Type:        money.TransactionTypeTransfer,
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: nil,
				},
			},
		}
		txBytes, err := (tx).MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{
			Version:          1,
			TransactionOrder: txBytes,
			ServerMetadata:   &types.ServerMetadata{},
		}
		txRecProof := &types.TxRecordProof{TxRecord: txRec, TxProof: &types.TxProof{Version: 1}}
		refNo := []byte{1, 2, 3, 4, 5}
		// txRec.ReferenceNumber == nil but refNo param != nil
		sum, err := transferredSum(txRecProof, nil, refNo, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)

		// txRec.ReferenceNumber != refNo (we add extra zero to the end)
		tx.ClientMetadata.ReferenceNumber = slices.Concat(refNo, []byte{0})
		sum, err = transferredSum(txRecProof, nil, refNo, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
	})

	t.Run("valid input but not transfer tx", func(t *testing.T) {
		// all money tx types other than TransactionTypeSplit and TransactionTypeTransfer should
		// be ignored ie cause no error but return zero as sum
		txTypes := []uint16{money.TransactionTypeSwapDC, money.TransactionTypeTransDC}
		tx := &types.TransactionOrder{Version: 1}
		txRec := &types.TransactionRecord{
			Version:          1,
			TransactionOrder: nil,
			ServerMetadata:   &types.ServerMetadata{},
		}
		for _, txt := range txTypes {
			tx.Payload = types.Payload{
				PartitionID: money.DefaultPartitionID,
				Type:        txt,
			}
			txBytes, err := tx.MarshalCBOR()
			require.NoError(t, err)
			txRec.TransactionOrder = txBytes
			txRecProof := &types.TxRecordProof{TxRecord: txRec, TxProof: &types.TxProof{Version: 1}}
			sum, err := transferredSum(txRecProof, nil, nil, tbLoader)
			require.NoError(t, err)
			require.Zero(t, sum)
		}
	})

	t.Run("txType and attributes do not match", func(t *testing.T) {
		txPayment := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: money.DefaultPartitionID,
				Type:        money.TransactionTypeSplit,
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		// txType is Split but use Transfer attributes!
		err := txPayment.SetAttributes(money.TransferAttributes{
			NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkHash),
			TargetValue:       100,
		})
		require.NoError(t, err)
		txBytes, err := txPayment.MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
		txRecProof := &types.TxRecordProof{TxRecord: txRec, TxProof: &types.TxProof{Version: 1}}

		sum, err := transferredSum(txRecProof, nil, nil, tbLoader)
		require.EqualError(t, err, `decoding split attributes: cbor: cannot unmarshal array into Go value of type money.SplitAttributes (cannot decode CBOR array to struct with different number of elements)`)
		require.Zero(t, sum)
	})

	tbSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	t.Run("transfer tx", func(t *testing.T) {
		refNo := []byte("reasons")
		txPayment := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: money.DefaultPartitionID,
				Type:        money.TransactionTypeTransfer,
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: slices.Clone(refNo),
				},
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		err = txPayment.SetAttributes(money.TransferAttributes{
			NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkHash),
			TargetValue:       100,
		})
		require.NoError(t, err)

		txBytes, err := txPayment.MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
		txRecProof := testblock.CreateTxRecordProof(t, txRec, tbSigner, testblock.WithPartitionID(money.DefaultPartitionID))
		// match without ref-no
		sum, err := transferredSum(txRecProof, pkHash, nil, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// match with ref-no
		sum, err = transferredSum(txRecProof, pkHash, refNo, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// different PKH, should get zero
		sum, err = transferredSum(txRecProof, []byte{1, 1, 1, 1, 1}, refNo, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 0, sum)
		// sum where ref-no is not set, should get zero as our transfer does have ref-no
		sum, err = transferredSum(txRecProof, pkHash, []byte{}, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
		// transfer does not verify
		errNOK := errors.New("this is bogus")
		tbNOK := &mockRootTrustBase{
			// need VerifyQuorumSignatures for verifying tx proofs
			verifyQuorumSignatures: func(data []byte, signatures map[string]hex.Bytes) (error, []error) { return errNOK, nil },
		}
		tbl := func(epoch uint64) (types.RootTrustBase, error) { return tbNOK, nil }
		sum, err = transferredSum(txRecProof, pkHash, nil, tbl)
		require.ErrorIs(t, err, errNOK)
		require.Zero(t, sum)
	})

	t.Run("split tx", func(t *testing.T) {
		refNo := []byte("reasons")
		txPayment := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				PartitionID: money.DefaultPartitionID,
				Type:        money.TransactionTypeSplit,
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: slices.Clone(refNo),
				},
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		err = txPayment.SetAttributes(money.SplitAttributes{
			TargetUnits: []*money.TargetUnit{
				{Amount: 10, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkHash)},
				{Amount: 50, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash([]byte("other guy"))},
				{Amount: 90, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(pkHash)},
			},
		})
		require.NoError(t, err)

		txBytes, err := txPayment.MarshalCBOR()
		require.NoError(t, err)
		txRec := &types.TransactionRecord{Version: 1, TransactionOrder: txBytes, ServerMetadata: &types.ServerMetadata{ActualFee: 25, SuccessIndicator: types.TxStatusSuccessful}}
		txRecProof := testblock.CreateTxRecordProof(t, txRec, tbSigner, testblock.WithPartitionID(money.DefaultPartitionID))

		// match without ref-no
		sum, err := transferredSum(txRecProof, pkHash, nil, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// match with ref-no
		sum, err = transferredSum(txRecProof, pkHash, refNo, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// PKH not in use by any units, should get zero
		sum, err = transferredSum(txRecProof, []byte{1, 1, 1, 1, 1}, refNo, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 0, sum)
		// the other guy
		sum, err = transferredSum(txRecProof, []byte("other guy"), nil, tbLoader)
		require.NoError(t, err)
		require.EqualValues(t, 50, sum)
		// sum where ref-no is not set, should get zero as our transfer does have ref-no
		sum, err = transferredSum(txRecProof, pkHash, []byte{}, tbLoader)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
		// transfer does not verify
		errNOK := errors.New("this is bogus")
		tbNOK := &mockRootTrustBase{
			// need VerifyQuorumSignatures for verifying tx proofs
			verifyQuorumSignatures: func(data []byte, signatures map[string]hex.Bytes) (error, []error) { return errNOK, nil },
		}
		tbl := func(epoch uint64) (types.RootTrustBase, error) { return tbNOK, nil }
		sum, err = transferredSum(txRecProof, pkHash, nil, tbl)
		require.ErrorIs(t, err, errNOK)
		require.Zero(t, sum)
	})
}
