package money

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

const dustTransferValue = 100

func TestModule_validateSwapTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("Ok", func(t *testing.T) {
		swapTx, swapAttr, authProof := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx))
	})
	t.Run("DC money supply < transaction target value", func(t *testing.T) {
		swapTx, swapAttr, authProof := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 99, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "insufficient DC-money supply")
	})
	t.Run("target unit does not exist", func(t *testing.T) {
		swapTx, swapAttr, authProof := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "target unit error: item FF does not exist: not found")
	})
	t.Run("DustTransfersInDescBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr, authProof := newDescBillOrderSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInEqualBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr, authProof := newEqualBillIdsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInvalidTargetPartitionID", func(t *testing.T) {
		swapTx, swapAttr, authProof := newSwapOrderWithInvalidTargetPartitionID(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer partition id is not money partition partition id: expected 1 vs provided 0")
	})
	t.Run("invalid target unit id", func(t *testing.T) {
		swapTx, swapAttr, authProof := newInvalidTargetUnitIDSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer order target unit id is not equal to swap transaction unit id")
	})
	t.Run("invalid target counter", func(t *testing.T) {
		swapTx, swapAttr, authProof := newInvalidTargetCounterSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer target counter is not equal to target unit counter: expected 0 vs provided 7")
	})
	t.Run("InvalidProofsNil", func(t *testing.T) {
		swapTx, swapAttr, authProof := newDcProofsNilSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transaction verification failed: failed to verify dust transfer at index 0: transaction proof is nil")
	})
	t.Run("InvalidEmptyDcProof", func(t *testing.T) {
		swapTx, swapAttr, authProof := newEmptyDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer proof is not valid at index 0: verify tx inclusion: failed to get unicity certificate: unicity certificate is nil")
	})
	t.Run("InvalidDcProofInvalid", func(t *testing.T) {
		swapTx, swapAttr, authProof := newInvalidDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), "dust transfer proof is not valid at index 0: verify tx inclusion: invalid unicity certificate: verifying unicity seal: verifying signatures: quorum not reached, signed_votes=0 quorum_threshold=1")
	})
	t.Run("owner proof error", func(t *testing.T) {
		swapTx, swapAttr, authProof := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID, &money.BillData{Value: 0, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(test.RandomBytes(10))}),
			withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: 1e8, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, authProof, exeCtx), `swap transaction predicate validation failed: predicate evaluated to "false"`)
	})
}

func TestModule_executeSwapTx(t *testing.T) {
	const dustAmount = uint64(10000)
	const targetBillValue = uint64(1)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	swapTx, swapAttr, authProof := newSwapDC(t, signer)
	module := newTestMoneyModule(t, verifier,
		withStateUnit(swapTx.UnitID, &money.BillData{Value: targetBillValue, Counter: 0, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}),
		withStateUnit(DustCollectorMoneySupplyID, &money.BillData{Value: dustAmount, Counter: 0, OwnerPredicate: DustCollectorPredicate}))
	exeCtx := testctx.NewMockExecutionContext(
		testctx.WithCurrentRound(6),
		testctx.WithData(util.Uint64ToBytes(dustTransferValue)),
	)
	sm, err := module.executeSwapTx(swapTx, swapAttr, authProof, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{swapTx.UnitID, DustCollectorMoneySupplyID}, sm.TargetUnits)
	u, err := module.state.GetUnit(swapTx.UnitID, false)
	require.NoError(t, err)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Owner(), templates.NewP2pkh256BytesFromKey(pubKey))
	require.EqualValues(t, bill.Value, targetBillValue+dustTransferValue)
	// counter was 0,
	require.EqualValues(t, bill.Counter, 1)
	require.EqualValues(t, bill.Locked, 0)
	// check dust bill as well
	d, err := module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	dustBill, ok := d.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, dustBill.Owner(), DustCollectorPredicate)
	require.EqualValues(t, dustBill.Value, dustAmount-dustTransferValue)
	require.EqualValues(t, dustBill.Locked, 0)
}

func createTransferDCTransactionRecord(t *testing.T, transferID []byte, attr *money.TransferDCAttributes) *types.TransactionRecord {
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeTransDC),
		testtransaction.WithUnitID(transferID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return transferDCRecord
}

func newInvalidTargetUnitIDSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: []byte{0},
		Value:        100,
		Counter:      6,
	})
	attr := &money.SwapDCAttributes{
		DustTransferProofs: []*types.TxRecordProof{
			testblock.CreateTxRecordProof(t, transferDCRecord, signer),
		},
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attr, authProof
}

func newInvalidTargetCounterSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferID := newBillID(1)
	swapID := newBillID(255)
	return createSwapDCTransactionOrder(t, signer, swapID, createTransferDCTransactionRecord(t, transferID, &money.TransferDCAttributes{
		TargetUnitID:      swapID,
		Value:             100,
		Counter:           6,
		TargetUnitCounter: 7,
	}))
}

func newDescBillOrderSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	// create swap transaction with two dust transfers in descending order of bill ids
	billIds := []types.UnitID{newBillID(2), newBillID(1)}
	swapId := newBillID(255)
	proofs := make([]*types.TxRecordProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		dustTransferRecord := createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		})
		proofs[i] = testblock.CreateTxRecordProof(t, dustTransferRecord, signer)
	}
	attr := &money.SwapDCAttributes{DustTransferProofs: proofs}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attr, authProof
}

func newEqualBillIdsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	// create swap transaction with two dust transfers with equal bill ids
	billIds := []types.UnitID{newBillID(1), newBillID(1)}
	swapId := newBillID(255)
	txrProofs := make([]*types.TxRecordProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		dustTransferRecord := createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        dustTransferValue,
			Counter:      6,
		})
		txrProofs[i] = testblock.CreateTxRecordProof(t, dustTransferRecord, signer)
	}
	attr := &money.SwapDCAttributes{DustTransferProofs: txrProofs}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attr, authProof
}

func newSwapOrderWithInvalidTargetPartitionID(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithPartitionID(0),
		testtransaction.WithTransactionType(money.TransactionTypeTransDC),
		testtransaction.WithUnitID(transferId),
		testtransaction.WithAttributes(&money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return createSwapDCTransactionOrder(t, signer, swapId, transferDCRecord)
}

func newDcProofsNilSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	proofs := []*types.TxRecordProof{
		{TxRecord: transferDCRecord, TxProof: nil},
	}
	attr := &money.SwapDCAttributes{DustTransferProofs: proofs}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attr, authProof
}

func newEmptyDcProofsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	proofs := []*types.TxRecordProof{
		{
			TxRecord: transferDCRecord,
			TxProof:  &types.TxProof{Version: 1, BlockHeaderHash: []byte{0}},
		},
	}
	attr := &money.SwapDCAttributes{DustTransferProofs: proofs}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attr, authProof
}

func newInvalidDcProofsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	UCSigner, _ := testsig.CreateSignerAndVerifier(t)
	txo, attr, _ := newSwapDC(t, UCSigner)

	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))

	return txo, attr, authProof
}

func newSwapDC(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	transferId := newBillID(1)
	swapId := []byte{255}
	return createSwapDCTransactionOrder(t, signer, swapId, createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        dustTransferValue,
		Counter:      0,
	}))
}

func createSwapDCTransactionOrder(t *testing.T, signer abcrypto.Signer, swapId []byte, transferDCRecords ...*types.TransactionRecord) (*types.TransactionOrder, *money.SwapDCAttributes, *money.SwapDCAuthProof) {
	var proofs []*types.TxRecordProof
	for _, dcTx := range transferDCRecords {
		proofs = append(proofs, testblock.CreateTxRecordProof(t, dcTx, signer))
	}
	attrs := &money.SwapDCAttributes{DustTransferProofs: proofs}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPartitionID(partitionIdentifier),
		testtransaction.WithTransactionType(money.TransactionTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attrs),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	authProof := &money.SwapDCAuthProof{OwnerProof: ownerProof}
	require.NoError(t, txo.SetAuthProof(authProof))
	return txo, attrs, authProof
}
