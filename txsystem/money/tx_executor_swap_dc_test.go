package money

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestModule_validateSwapTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("Ok", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.NoError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx))
	})
	t.Run("DC money supply < tx target value", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 99, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "insufficient DC-money supply")
	})
	t.Run("target unit does not exist", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "target unit error: item FF does not exist: not found")
	})
	t.Run("InvalidTargetValue", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetValueSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "target value must be equal to the sum of dust transfer values: expected 90 vs provided 100")
	})
	t.Run("DustTransfersInDescBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr := newDescBillOrderSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInEqualBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr := newEqualBillIdsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInvalidTargetSystemID", func(t *testing.T) {
		swapTx, swapAttr := newSwapOrderWithInvalidTargetSystemID(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer system id is not money partition system id: expected 00000001 vs provided 00000000")
	})
	t.Run("invalid target unit id", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetUnitIDSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer order target unit id is not equal to swap tx unit id")
	})
	t.Run("invalid target counter", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetCounterSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer target counter is not equal to target unit counter: expected 0 vs provided 7")
	})
	t.Run("InvalidProofsNil", func(t *testing.T) {
		swapTx, swapAttr := newDcProofsNilSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "invalid count of proofs: expected 1 vs provided 0")
	})
	t.Run("InvalidEmptyDcProof", func(t *testing.T) {
		swapTx, swapAttr := newEmptyDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "proof is not valid: invalid unicity certificate: unicity certificate validation failed: unicity certificate is nil")
	})
	t.Run("InvalidDcProofInvalid", func(t *testing.T) {
		swapTx, swapAttr := newInvalidDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "proof is not valid: invalid unicity certificate: unicity seal signature validation failed: quorum not reached, signed_votes=0 quorum_threshold=1")
	})
	t.Run("owner proof error", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(test.RandomBytes(10)), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), `swap tx predicate validation failed: predicate evaluated to "false"`)
	})
}

func TestModule_executeSwapTx(t *testing.T) {
	const dustAmount = uint64(10000)
	const targetBillValue = uint64(1)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	swapTx, swapAttr := newSwapDC(t, signer)
	module := newTestMoneyModule(t, verifier,
		withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: targetBillValue, T: 0, Counter: 0}),
		withStateUnit(DustCollectorMoneySupplyID, DustCollectorPredicate, &money.BillData{V: dustAmount, T: 0, Counter: 0}))
	exeCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(6))
	sm, err := module.executeSwapTx(swapTx, swapAttr, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{swapTx.UnitID(), DustCollectorMoneySupplyID}, sm.TargetUnits)
	u, err := module.state.GetUnit(swapTx.UnitID(), false)
	require.NoError(t, err)
	require.EqualValues(t, u.Bearer(), templates.NewP2pkh256BytesFromKey(pubKey))
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.V, targetBillValue+swapAttr.TargetValue)
	// counter was 0,
	require.EqualValues(t, bill.Counter, 1)
	require.EqualValues(t, bill.T, exeCtx.CurrentRound())
	require.EqualValues(t, bill.Locked, 0)
	// check dust bill as well
	d, err := module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	require.EqualValues(t, d.Bearer(), DustCollectorPredicate)
	dustBill, ok := d.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, dustBill.V, dustAmount-swapAttr.TargetValue)
	// counter was 0,
	require.EqualValues(t, dustBill.Counter, 1)
	require.EqualValues(t, dustBill.T, exeCtx.CurrentRound())
	require.EqualValues(t, dustBill.Locked, 0)
}

func createTransferDCTransactionRecord(t *testing.T, transferID []byte, attr *money.TransferDCAttributes) *types.TransactionRecord {
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeTransDC),
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

func newInvalidTargetValueSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        90,
		Counter:      6,
	})
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      []*types.TransactionRecord{transferDCRecord},
		DcTransferProofs: []*types.TxProof{nil},
		TargetValue:      100,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newInvalidTargetUnitIDSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: []byte{0},
		Value:        100,
		Counter:      6,
	})
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      []*types.TransactionRecord{transferDCRecord},
		DcTransferProofs: []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
		TargetValue:      100,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newInvalidTargetCounterSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferID := newBillID(1)
	swapID := newBillID(255)
	return createSwapDCTransactionOrder(t, signer, swapID, createTransferDCTransactionRecord(t, transferID, &money.TransferDCAttributes{
		TargetUnitID:      swapID,
		Value:             100,
		Counter:           6,
		TargetUnitCounter: 7,
	}))
}

func newDescBillOrderSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	// create swap tx with two dust transfers in descending order of bill ids
	billIds := []types.UnitID{newBillID(2), newBillID(1)}
	swapId := newBillID(255)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		transferIds[i] = billIds[i]
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      dcTransfers,
		DcTransferProofs: proofs,
		TargetValue:      200,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newEqualBillIdsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	// create swap tx with two dust transfers with equal bill ids
	billIds := []types.UnitID{newBillID(1), newBillID(1)}
	swapId := newBillID(255)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		transferIds[i] = billIds[i]
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      dcTransfers,
		DcTransferProofs: proofs,
		TargetValue:      200,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newSwapOrderWithInvalidTargetSystemID(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID(0),
		testtransaction.WithPayloadType(money.PayloadTypeTransDC),
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

func newDcProofsNilSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      []*types.TransactionRecord{transferDCRecord},
		DcTransferProofs: nil,
		TargetValue:      100,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newEmptyDcProofsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      []*types.TransactionRecord{transferDCRecord},
		DcTransferProofs: []*types.TxProof{{BlockHeaderHash: []byte{0}}},
		TargetValue:      100,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newInvalidDcProofsSwap(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	UCSigner, _ := testsig.CreateSignerAndVerifier(t)
	txo, attr := newSwapDC(t, UCSigner)
	// newSwapDC uses passed in signer for both trustbase and txo
	// so we need to reset owner proof to the correct tx signer
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attr
}

func newSwapDC(t *testing.T, signer abcrypto.Signer) (*types.TransactionOrder, *money.SwapDCAttributes) {
	transferId := newBillID(1)
	swapId := []byte{255}

	return createSwapDCTransactionOrder(t, signer, swapId, createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      0,
	}))
}

func createSwapDCTransactionOrder(t *testing.T, signer abcrypto.Signer, swapId []byte, transferDCRecords ...*types.TransactionRecord) (*types.TransactionOrder, *money.SwapDCAttributes) {
	var proofs []*types.TxProof
	for _, dcTx := range transferDCRecords {
		proofs = append(proofs, testblock.CreateProof(t, dcTx, signer))
	}
	attrs := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      transferDCRecords,
		DcTransferProofs: proofs,
		TargetValue:      100,
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(attrs),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo, attrs
}
