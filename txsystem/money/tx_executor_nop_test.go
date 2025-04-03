package money

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestModule_validateNopTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	counter := uint64(1)

	t.Run("ok with bill unit data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})
	t.Run("ok with fcr unit data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})
	t.Run("ok with dummy unit data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withDummyUnit(unitID))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})
	t.Run("nok with invalid counter for normal unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Counter: 2}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorIs(t, module.validateNopTx(tx, attr, authProof, exeCtx), ErrInvalidCounter)
	})
	t.Run("nok with invalid counter for fcr unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: 2}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorIs(t, module.validateNopTx(tx, attr, authProof, exeCtx), ErrInvalidCounter)
	})
	t.Run("nok with invalid counter for dummy unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withDummyUnit(unitID))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter must be nil for dummy unit data")
	})
	t.Run("nok with nil counter for normal unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorIs(t, module.validateNopTx(tx, attr, authProof, exeCtx), ErrInvalidCounter)
	})
	t.Run("nok with nil counter for fcr unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorIs(t, module.validateNopTx(tx, attr, authProof, exeCtx), ErrInvalidCounter)
	})
	t.Run("nok with invalid auth proof for normal unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "verify owner: evaluating owner predicate")
	})
	t.Run("nok with invalid auth proof for fcr unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "verify owner: evaluating owner predicate")
	})
	t.Run("nok with invalid auth proof for dummy unit", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withDummyUnit(unitID))
		tx := createTx(unitID, fcrID, nop.TransactionTypeNOP)
		attr := &nop.Attributes{}
		require.NoError(t, tx.SetAttributes(attr))
		authProof := &nop.AuthProof{OwnerProof: []byte{1}} // tx targeting dummy unit cannot contain owner proof
		require.NoError(t, tx.SetAuthProof(authProof))

		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "nop transaction targeting dummy unit cannot contain owner proof")
	})
}

func TestModule_executeNopTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("normal unit ok - counter is incremented", func(t *testing.T) {
		counter := uint64(1)
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify only counter was incremented and all other fields are unchanged
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		bill, ok := u.Data().(*money.BillData)
		require.True(t, ok)
		require.EqualValues(t, 2, bill.Counter)
		require.EqualValues(t, 10, bill.Value)
		require.Nil(t, bill.OwnerPredicate)
	})
	t.Run("fcr unit ok - counter is incremented", func(t *testing.T) {
		counter := uint64(1)
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify only counter was incremented and all other fields are unchanged
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		fcr, ok := u.Data().(*fcsdk.FeeCreditRecord)
		require.True(t, ok)
		require.EqualValues(t, 2, fcr.Counter)
		require.EqualValues(t, 10, fcr.Balance)
		require.Nil(t, fcr.OwnerPredicate)
	})
	t.Run("dummy unit ok - nothing is changed", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withDummyUnit(unitID))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify nothing is changed
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.Nil(t, u.Data())
	})
}

func createNopTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter *uint64) (*types.TransactionOrder, *nop.Attributes, *nop.AuthProof) {
	tx := createTx(fromID, fcrID, nop.TransactionTypeNOP)
	attr := &nop.Attributes{
		Counter: counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &nop.AuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}
