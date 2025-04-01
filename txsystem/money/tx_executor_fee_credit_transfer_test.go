package money

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestModule_validateTransferFCTx(t *testing.T) {
	const counter = uint64(4)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	authProof := &fcsdk.TransferFeeCreditAuthProof{OwnerProof: nil}

	t.Run("Ok", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx))
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &fcsdk.FeeCreditRecord{Balance: 101, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "invalid unit type")
	})
	t.Run("err - bill not found", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "unit not found 000000000000000000000000000000000000000000000000000000000000000001")
	})
	t.Run("err - TargetPartitionID is zero", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, testutils.NewTransferFCAttr(t, signer, testutils.WithTargetPartitionID(0)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "TargetPartitionID is empty")
	})
	t.Run("err - TargetRecordID is nil", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, testutils.NewTransferFCAttr(t, signer, testutils.WithTargetRecordID(nil)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "TargetRecordID is empty")
	})
	t.Run("err - invalid amount", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, testutils.NewTransferFCAttr(t, signer, testutils.WithAmount(102)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "the amount to transfer cannot exceed the value of the bill")
	})
	t.Run("err - invalid fee", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, testutils.NewTransferFCAttr(t, signer, testutils.WithAmount(1)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "the transaction max fee cannot exceed the transferred amount")
	})
	t.Run("err - invalid counter", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, testutils.NewTransferFCAttr(t, signer, testutils.WithCounter(counter+10)))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("err - FCR ID is set", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "fee transaction cannot contain fee credit reference")
	})
	t.Run("err - fee proof exists", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil,
			testtransaction.WithFeeProof([]byte{0, 0, 0, 0}))
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), "fee transaction cannot contain fee authorization proof")
	})
	t.Run("err - bearer predicate error", func(t *testing.T) {
		tx := testutils.NewTransferFC(t, signer, nil)
		attr := &fcsdk.TransferFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID, &money.BillData{Value: 101, Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferFCTx(tx, attr, authProof, exeCtx), `verify owner proof: predicate evaluated to "false"`)
	})
}

func TestModule_executeTransferFCTx(t *testing.T) {
	const counter = uint64(4)
	const value = uint64(101)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	tx := testutils.NewTransferFC(t, signer, nil)
	attr := &fcsdk.TransferFeeCreditAttributes{Amount: 10}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	module := newTestMoneyModule(t, verifier,
		withStateUnit(tx.UnitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
	exeCtx := testctx.NewMockExecutionContext()
	authProof := &fcsdk.TransferFeeCreditAuthProof{OwnerProof: nil}
	sm, err := module.executeTransferFCTx(tx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{tx.UnitID}, sm.TargetUnits)
	// validate changes
	u, err := module.state.GetUnit(tx.UnitID, false)
	require.NoError(t, err)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Owner(), templates.AlwaysTrueBytes())
	// target bill is credited correct amount
	require.EqualValues(t, bill.Value, value-attr.Amount)
	// counter is incremented
	require.EqualValues(t, bill.Counter, counter+1)
}
