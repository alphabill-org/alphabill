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
	"github.com/stretchr/testify/require"
)

func TestModule_validateUnlockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("ok", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1}))
		lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.NoError(t, module.validateUnlockTx(lockTx, attr, exeCtx))
	})
	t.Run("unit not found", func(t *testing.T) {
		module := newTestMoneyModule(t, verifier)
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		lockTx, _ := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, nil, exeCtx), "unlock tx: get unit error: item 000000000000000000000000000000000000000000000000000000000001020300 does not exist: not found")
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}))
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "unlock tx: invalid unit type")
	})
	t.Run("bill is already unlocked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 0}))
		lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "bill is already unlocked")
	})
	t.Run("invalid counter", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: 10, Locked: 1, Counter: 1}))
		lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("bearer predicate error", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{1, 2, 3})
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysFalseBytes(), &money.BillData{V: 10, Locked: 1, Counter: 0}))
		lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
		exeCtx := &txsystem.TxExecutionContext{}
		require.EqualError(t, module.validateUnlockTx(lockTx, attr, exeCtx), `predicate evaluated to "false"`)
	})
}

func TestModule_executeUnlockTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const value = uint64(10)
	const counter = uint64(1)
	unitID := money.NewBillID(nil, []byte{1, 2, 3})
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Locked: 1, Counter: counter}))
	lockTx, attr := createUnlockTx(t, unitID, fcrID, 0)
	exeCtx := &txsystem.TxExecutionContext{}
	sm, err := module.executeUnlockTx(lockTx, attr, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.EqualValues(t, u.Bearer(), templates.AlwaysTrueBytes())
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.V, value)
	// counter is incremented
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, exeCtx.CurrentBlockNr)
	require.EqualValues(t, bill.Locked, 0)
}
