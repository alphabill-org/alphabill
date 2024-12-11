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

func TestFeeCredit_validateUnlockFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("ok", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.NoError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase)
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			fmt.Sprintf("get unit error: get fcr unit error: item %s does not exist: not found", fcrID))
	})
	t.Run("unit id type part is not fee credit record", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, testutils.NewUnlockFCAttr(), testtransaction.WithUnitID(
			types.NewUnitID(33, nil, []byte{1}, []byte{0xfe})),
		)
		feeModule := newTestFeeModule(t, trustBase, withFeeCreditType([]byte{0xff}), withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"get unit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("invalid unit data type", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID, &testData{}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"get unit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("FCR is already unlocked", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 0}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"fee credit record is already unlocked")
	})
	t.Run("invalid counter", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, testutils.NewUnlockFCAttr(testutils.WithUnlockFCCounter(3)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"the transaction counter does not equal with the fee credit record counter: got 3 expected 4")
	})
	t.Run("max fee exceeds balance", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee credit reference")
	})
	t.Run("fee proof is not nil", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, signer, nil, testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.UnlockFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee authorization proof")
	})
}

func TestFeeCredit_executeUnlockFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	tx := testutils.NewUnlockFC(t, signer, nil)
	initialFcr := &fc.FeeCreditRecord{Balance: 50, Counter: 4, Locked: 1}
	feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID, initialFcr))
	var attr fc.UnlockFeeCreditAttributes
	require.NoError(t, tx.UnmarshalAttributes(&attr))
	var authProof fc.UnlockFeeCreditAuthProof
	require.NoError(t, tx.UnmarshalAuthProof(&authProof))
	execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
	require.NoError(t, feeModule.validateUnlockFC(tx, &attr, &authProof, execCtx))
	sm, err := feeModule.executeUnlockFC(tx, &attr, &authProof, execCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	fcrUnit, err := feeModule.state.GetUnit(tx.UnitID, false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, uint64(0), fcr.Locked)
	require.Equal(t, 50-sm.ActualFee, fcr.Balance) //-fee
	require.Equal(t, uint64(0), fcr.MinLifetime)
	// counter is incremented
	require.NotEqual(t, initialFcr.Counter, fcr.Counter)

}
