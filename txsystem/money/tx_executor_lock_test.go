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
	"github.com/stretchr/testify/require"
)

func TestModule_validateLockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("ok", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateLockTx(lockTx, attr, authProof, exeCtx))
	})
	t.Run("unit not found", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier)
		lockTx, _, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateLockTx(lockTx, nil, authProof, exeCtx), "lock transaction: get unit error: item 000000000000000000000000000000000000000000000000000000000001020300 does not exist: not found")
	})
	t.Run("invalid unit type", func(t *testing.T) {
		unitID := money.NewFeeCreditRecordID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateLockTx(lockTx, attr, authProof, exeCtx), "lock transaction: invalid unit type")
	})
	t.Run("bill is already locked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateLockTx(lockTx, attr, authProof, exeCtx), "bill is already locked")
	})
	t.Run("zero lock value", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 0}))
		lockTx := createTx(unitID, fcrID, money.TransactionTypeLock)
		lockTxAttr := &money.LockAttributes{
			LockStatus: 0,
			Counter:    0,
		}
		require.NoError(t, lockTx.SetAttributes(lockTxAttr))
		authProof := &money.LockAuthProof{}
		require.NoError(t, lockTx.SetAuthProof(authProof))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateLockTx(lockTx, lockTxAttr, authProof, exeCtx), "invalid lock status: expected non-zero value, got zero value")
	})
	t.Run("invalid counter", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Counter: 1}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateLockTx(lockTx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("invalid owner", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysFalseBytes(), &money.BillData{V: 10}))
		lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateLockTx(lockTx, attr, authProof, exeCtx), "evaluating owner predicate")
	})
}

func TestModule_executeLockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const value = uint64(10)
	const counter = uint64(0)
	unitID := money.NewBillID(nil, []byte{1, 2, 3})
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
	lockTx, attr, authProof := createLockTx(t, unitID, fcrID, 0)
	exeCtx := testctx.NewMockExecutionContext()
	sm, err := module.executeLockTx(lockTx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.EqualValues(t, u.Owner(), templates.AlwaysTrueBytes())
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.V, value)
	// counter was 0,
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, exeCtx.CurrentRound())
	require.EqualValues(t, bill.Locked, 1)
}
