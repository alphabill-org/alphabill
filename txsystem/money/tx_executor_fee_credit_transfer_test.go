package money

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestModule_validateTransferFCTx(t *testing.T) {
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
	t.Run("unit is not bill data", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 101}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), "invalid unit type")
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
	t.Run("err - bearer predicate error", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysFalseBytes(), &money.BillData{V: 101, Counter: counter}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateTransferFCTx(tx, attr, exeCtx), `verify owner proof: predicate evaluated to "false"`)
	})
}

func TestModule_executeTransferFCTx(t *testing.T) {
	const counter = uint64(4)
	const value = uint64(101)
	_, verifier := testsig.CreateSignerAndVerifier(t)
	tx := testutils.NewTransferFC(t, nil)
	attr := &fcsdk.TransferFeeCreditAttributes{Amount: 10}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	module := newTestMoneyModule(t, verifier,
		withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
	exeCtx := &txsystem.TxExecutionContext{}
	sm, err := module.executeTransferFCTx(tx, attr, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{tx.UnitID()}, sm.TargetUnits)
	// validate changes
	u, err := module.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	require.EqualValues(t, u.Bearer(), templates.AlwaysTrueBytes())
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	// target bill is credited correct amount
	require.EqualValues(t, bill.V, value-attr.Amount)
	// counter is incremented
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, exeCtx.CurrentBlockNr)
	require.EqualValues(t, bill.Locked, 0)
}
