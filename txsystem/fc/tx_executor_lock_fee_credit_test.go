package fc

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestFeeCredit_validateLockFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("ok", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.NoError(t, feeModule.validateLockFC(tx, &attr, execCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase)
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			fmt.Sprintf("get unit error: get fcr unit error: item %s does not exist: not found", fcrID),
		)
	})
	t.Run("unit id type part is not fee credit record", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, testutils.NewLockFCAttr(), testtransaction.WithUnitID(
			types.NewUnitID(33, nil, []byte{1}, []byte{0xfe})),
		)
		feeModule := newTestFeeModule(t, trustBase, withFeeCreditType([]byte{0xff}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"get unit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("invalid unit data type", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &testData{}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"get unit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("FCR is already locked", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"fee credit record is already locked")
	})
	t.Run("lock status is zero", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, testutils.NewLockFCAttr(testutils.WithLockStatus(0)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 0}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"lock status must be non-zero value")
	})
	t.Run("invalid counter", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, testutils.NewLockFCAttr(testutils.WithLockFCCounter(3)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"the transaction counter does not equal with the fee credit record counter: got 3 expected 4")
	})
	t.Run("max fee exceeds balance", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("fee proof is not nil", func(t *testing.T) {
		tx := testutils.NewLockFC(t, signer, nil, testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.LockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateLockFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee authorization proof")
	})
}

func TestFeeCredit_executeLockFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	tx := testutils.NewLockFC(t, signer, nil)
	initialFcr := &fc.FeeCreditRecord{Balance: 50, Counter: 4}
	feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, initialFcr))
	var attr fc.LockFeeCreditAttributes
	require.NoError(t, tx.UnmarshalAttributes(&attr))
	execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
	require.NoError(t, feeModule.validateLockFC(tx, &attr, execCtx))
	sm, err := feeModule.executeLockFC(tx, &attr, execCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	fcrUnit, err := feeModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, uint64(1), fcr.Locked)
	require.Equal(t, 50-sm.ActualFee, fcr.Balance) //-fee
	require.Equal(t, uint64(0), fcr.Timeout)
	// counter is incremented
	require.NotEqual(t, initialFcr.Counter, fcr.Counter)
}
