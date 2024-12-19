package money

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
)

func TestModule_validateUnlockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("ok", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Locked: 1, OwnerPredicate: templates.AlwaysTrueBytes()}))
		lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateUnlockTx(lockTx, attr, authProof, exeCtx))
	})
	t.Run("unit not found", func(t *testing.T) {
		module := newTestMoneyModule(t, verifier)
		unitID := moneyid.NewBillID(t)
		lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, authProof, exeCtx), fmt.Sprintf("unlock transaction: get unit error: item %s does not exist: not found", unitID))
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, authProof, exeCtx), "unlock transaction: invalid unit type")
	})
	t.Run("bill is already unlocked", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Locked: 0, OwnerPredicate: templates.AlwaysTrueBytes()}))
		lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, authProof, exeCtx), "bill is already unlocked")
	})
	t.Run("invalid counter", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, Locked: 1, Counter: 1, OwnerPredicate: templates.AlwaysTrueBytes()}))
		lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("invalid owner", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: 10, OwnerPredicate: templates.AlwaysFalseBytes()}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateLockTx(lockTx, attr, authProof, exeCtx), "evaluating owner predicate")
	})
}

func TestModule_executeUnlockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const value = uint64(10)
	const counter = uint64(1)
	unitID := moneyid.NewBillID(t)
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Locked: 1, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
	lockTx, attr, authProof := createUnlockTx(t, unitID, fcrID, 0)
	exeCtx := testctx.NewMockExecutionContext()
	sm, err := module.executeUnlockTx(lockTx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Owner(), templates.AlwaysTrueBytes())
	require.EqualValues(t, bill.Value, value)
	// counter is incremented
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.Locked, 0)
}
