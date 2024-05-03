package money

import (
	"math"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.TransferAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 6},
			res:  nil,
		},
		{
			name: "LockedBill",
			bd:   &money.BillData{Locked: 1, V: 100, Counter: 6},
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 6},
			res:  ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 101, Counter: 6},
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidCounter",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 5},
			res:  ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnyTransfer(tt.bd, tt.attr.Counter, tt.attr.TargetValue)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestTransferDC(t *testing.T) {
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.TransferDCAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Counter:      6,
			},
			res: nil,
		},
		{
			name: "LockedBill",
			bd:   &money.BillData{Locked: 1, V: 100, Counter: 6},
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Counter:      6,
			},
			res: ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Counter:      6,
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidCounter",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Counter:      7,
			},
			res: ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransferDC(tt.bd, tt.attr)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	const counter = uint64(6)
	const billValue = uint64(100)
	t.Run("ok - 2-way split", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50, // - Amount split
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateSplitTx(tx, attr, exeCtx))
	})
	t.Run("ok - 3-way split", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			billValue-10-10, // two additional bills with value 10 are created
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateSplitTx(tx, attr, exeCtx))
	})
	t.Run("err - bill not found", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50,
			counter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "item 000000000000000000000000000000000000000000000000000000000000000200 does not exist: not found")
	})
	t.Run("err - bill locked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{Locked: 1, V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "bill is locked")
	})
	t.Run("err - invalid counter", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 20, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-20,
			counter+1)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("err - target units empty", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "target units are empty")
	})
	t.Run("err - target unit is nil", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{nil},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "target unit is nil at index 0")
	})
	t.Run("err - target unit amount is 0", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 0, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "target unit amount is zero at index 0")
	})
	t.Run("err - target unit owner condition is empty", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{{Amount: 1, OwnerCondition: []byte{}}},
			billValue-1,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "target unit owner condition is empty at index 0")
	})
	t.Run("err - target unit amount overflow", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{
				{Amount: math.MaxUint64, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 1, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "failed to add target unit amounts: uint64 sum overflow: [18446744073709551615 1]")
	})
	t.Run("err - remaining value is zero", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{
				{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			0,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "remaining value is zero")
	})
	t.Run("err - amount plus remaining value is less than bill value", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			79,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx),
			"the sum of the values to be transferred plus the remaining value must equal the value of the bill; sum=20 remainingValue=79 billValue=100")
	})
	t.Run("err - amount plus remaining value is less than bill value", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			81,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx),
			"the sum of the values to be transferred plus the remaining value must equal the value of the bill; sum=20 remainingValue=81 billValue=100")
	})
}

func TestSwap(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("Ok", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx))
	})
	t.Run("DC money supply < tx target value", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 99, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "insufficient DC-money supply")
	})
	t.Run("target unit does not exist", func(t *testing.T) {
		swapTx, swapAttr := newSwapDC(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "target unit error: item FF does not exist: not found")
	})
	t.Run("InvalidTargetValue", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetValueSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "target value must be equal to the sum of dust transfer values: expected 90 vs provided 100")
	})
	t.Run("DustTransfersInDescBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr := newDescBillOrderSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInEqualBillIdOrder", func(t *testing.T) {
		swapTx, swapAttr := newEqualBillIdsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer orders are not listed in strictly increasing order of bill identifiers")
	})
	t.Run("DustTransfersInvalidTargetSystemID", func(t *testing.T) {
		swapTx, swapAttr := newSwapOrderWithInvalidTargetSystemID(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer system id is not money partition system id: expected 00000001 vs provided 00000000")
	})
	t.Run("invalid target unit id", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetUnitIDSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer order target unit id is not equal to swap tx unit id")
	})
	t.Run("invalid target counter", func(t *testing.T) {
		swapTx, swapAttr := newInvalidTargetCounterSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "dust transfer target counter is not equal to target unit counter: expected 0 vs provided 7")
	})
	t.Run("InvalidProofsNil", func(t *testing.T) {
		swapTx, swapAttr := newDcProofsNilSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "invalid count of proofs: expected 1 vs provided 0")
	})
	t.Run("InvalidEmptyDcProof", func(t *testing.T) {
		swapTx, swapAttr := newEmptyDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "proof is not valid: invalid unicity certificate: unicity certificate validation failed: unicity certificate is nil")
	})
	t.Run("InvalidDcProofInvalid", func(t *testing.T) {
		swapTx, swapAttr := newInvalidDcProofsSwap(t, signer)
		module := newTestMoneyModule(t, verifier,
			withStateUnit(swapTx.UnitID(), templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: 0, T: 0, Counter: 0}),
			withStateUnit(DustCollectorMoneySupplyID, nil, &money.BillData{V: 1e8, T: 0, Counter: 0}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateSwapTx(swapTx, swapAttr, exeCtx), "proof is not valid: invalid unicity certificate: unicity seal signature validation failed: invalid unicity seal signature, verification failed")
	})
}

func TestTransferFC(t *testing.T) {
	const counter = uint64(4)
	_, verifier := testsig.CreateSignerAndVerifier(t)

	t.Run("Ok", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateTransferFCTx(tx, attr, exeCtx))
	})
	t.Run("err - locked bill", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{Locked: 1, V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "bill is locked")
	})
	t.Run("err - bill not found", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "unit not found 0000000000000000000000000000000000000000000000000000000000000001FF")
	})
	t.Run("err - TargetSystemIdentifier is zero", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetSystemID(0)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "TargetSystemIdentifier is empty")
	})
	t.Run("err - TargetRecordID is nil", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID(nil)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "TargetRecordID is empty")
	})
	t.Run("err - AdditionTime invalid", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(
			testutils.WithEarliestAdditionTime(2),
			testutils.WithLatestAdditionTime(1)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "EarliestAdditionTime is greater than LatestAdditionTime")
	})
	t.Run("err - invalid amount", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(102)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "the amount to transfer cannot exceed the value of the bill")
	})
	t.Run("err - invalid fee", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(1)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "the transaction max fee cannot exceed the transferred amount")
	})
	t.Run("err - invalid counter", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithCounter(counter+10)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("err - FCR ID is set", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "fee tx cannot contain fee credit reference")
	})
	t.Run("err - fee proof exists", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil,
			testtransaction.WithFeeProof([]byte{0, 0, 0, 0}))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "fee tx cannot contain fee authorization proof")
	})
}

func TestReclaimFC(t *testing.T) {
	const (
		amount  = uint64(100)
		counter = uint64(4)
	)
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	t.Run("Ok", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateReclaimFCTx(tx, attr, exeCtx))
	})
	t.Run("Bill is missing", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx),
			"get unit error: item 0000000000000000000000000000000000000000000000000000000000000001FF does not exist: not found")
	})
	t.Run("Fee credit record exists in tx", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "fee tx cannot contain fee credit reference")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithFeeProof([]byte{0, 0, 0, 0}))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "fee tx cannot contain fee authorization proof")
	})
	t.Run("Invalid target unit", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithUnitID(money.NewFeeCreditRecordID(nil, []byte{2})))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid target unit")
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer,
			testutils.NewReclaimFCAttr(t, signer,
				testutils.WithReclaimFCClosureTx(
					&types.TransactionRecord{
						TransactionOrder: testutils.NewCloseFC(t,
							testutils.NewCloseFCAttr(
								testutils.WithCloseFCAmount(2),
								testutils.WithCloseFCTargetUnitCounter(counter),
							),
						),
						ServerMetadata: &types.ServerMetadata{ActualFee: 10},
					},
				),
			),
		)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "the transaction fees cannot exceed the transferred value")
	})
	t.Run("Invalid target unit counter", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter + 1}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid target unit counter")
	})
	t.Run("Invalid counter", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer, testutils.WithReclaimFCCounter(counter+1)))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer,
			testutils.WithReclaimFCClosureProof(newInvalidProof(t, signer))))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid proof: proof block hash does not match to block hash in unicity certificate")
	})
}

func createMoneyModule(t *testing.T) *Module {
	options, err := defaultOptions()
	require.NoError(t, err)
	options.state = state.NewEmptyState()
	module, err := NewMoneyModule(options)
	require.NoError(t, err)
	return module
}

func TestLockTx(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10})))
		lockTx, attr := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateLockTx(lockTx, attr, exeCtx))
	})
	t.Run("attr is nil", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10})))
		lockTx, _ := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, nil, exeCtx), "tx attributes is nil")
	})
	t.Run("unit not found", func(t *testing.T) {
		module := createMoneyModule(t)
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		lockTx, _ := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, nil, exeCtx), "lock tx: get unit error: item 000000000000000000000000000000000000000000000000000000000001020300 does not exist: not found")
	})
	t.Run("bill is already locked", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1})))
		lockTx, attr := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, attr, exeCtx), "bill is already locked")
	})
	t.Run("zero lock value", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 0})))
		lockTx := createTx(unitID, money.PayloadTypeLock)
		lockTxAttr := &money.LockAttributes{
			LockStatus: 0,
			Counter:    0,
		}
		rawBytes, err := types.Cbor.Marshal(lockTxAttr)
		require.NoError(t, err)
		lockTx.Payload.Attributes = rawBytes
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, lockTxAttr, exeCtx), "invalid lock status: expected non-zero value, got zero value")
	})
	t.Run("invalid counter", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Counter: 1})))
		lockTx, attr := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("invalid unit type", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewFeeCreditRecordID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10})))
		lockTx, attr := createLockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateLockTx(lockTx, attr, exeCtx), "lock tx: invalid unit type")
	})
}

func TestUnlockTx(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1})))
		lockTx, attr := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateUnlockTx(lockTx, attr, exeCtx))
	})
	t.Run("attr is nil", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1})))
		lockTx, _ := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, nil, exeCtx), "tx attributes is nil")
	})
	t.Run("unit not found", func(t *testing.T) {
		module := createMoneyModule(t)
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		lockTx, _ := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, nil, exeCtx), "unlock tx: get unit error: item 000000000000000000000000000000000000000000000000000000000001020300 does not exist: not found")
	})
	t.Run("bill is already unlocked", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 0})))
		lockTx, attr := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "bill is already unlocked")
	})
	t.Run("invalid counter", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1, Counter: 1})))
		lockTx, attr := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("invalid unit type", func(t *testing.T) {
		module := createMoneyModule(t)
		s := module.state
		unitID := money.NewFeeCreditRecordID(nil, []byte{1, 2, 3})
		require.NoError(t, s.Apply(state.AddUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10})))
		lockTx, attr := createUnlockTx(t, unitID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "unlock tx: invalid unit type")
	})
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

func newBillData(v uint64, counter uint64) *money.BillData {
	return &money.BillData{V: v, Counter: counter}
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	attr := testutils.NewDefaultReclaimFCAttr(t, signer)
	attr.CloseFeeCreditProof.BlockHeaderHash = []byte("invalid hash")
	return attr.CloseFeeCreditProof
}

type moneyModuleOption func(m *Module) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) moneyModuleOption {
	return func(m *Module) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func newTestMoneyModule(t *testing.T, verifier abcrypto.Verifier, opts ...moneyModuleOption) *Module {
	module := defaultMoneyModule(t, verifier)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}

func defaultMoneyModule(t *testing.T, verifier abcrypto.Verifier) *Module {
	// NB! using the same pubkey for trustbase and unit bearer! TODO: use different keys...
	options, err := defaultOptions()
	require.NoError(t, err)
	options.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	options.state = state.NewEmptyState()
	module, err := NewMoneyModule(options)
	require.NoError(t, err)
	return module
}
