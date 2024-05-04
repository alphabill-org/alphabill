package fc

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestFeeCredit_validateUnlockFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)

	t.Run("ok", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.NoError(t, feeModule.validateUnlockFC(tx, &attr, execCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase)
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"get unit error: get fcr unit error: item 0000000000000000000000000000000000000000000000000000000000000001FF does not exist: not found")
	})
	t.Run("unit id type part is not fee credit record", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, testutils.NewUnlockFCAttr(), testtransaction.WithUnitID(
			types.NewUnitID(33, nil, []byte{1}, []byte{0xfe})),
		)
		feeModule := newTestFeeModule(t, trustBase, withFeeCreditType([]byte{0xff}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"get unit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("invalid unit data type", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &testData{}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"get unit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("FCR is already unlocked", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 0}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"fee credit record is already unlocked")
	})
	t.Run("invalid backlink", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, testutils.NewUnlockFCAttr(testutils.WithUnlockFCBacklink([]byte{3})))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"the transaction backlink does not match with fee credit record backlink: got 03 expected 04")
	})
	t.Run("max fee exceeds balance", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("fee proof is not nil", func(t *testing.T) {
		tx := testutils.NewUnlockFC(t, nil, testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil,
				&fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}))
		var attr fc.UnlockFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateUnlockFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee authorization proof")
	})
}

func TestFeeCredit_executeUnlockFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	tx := testutils.NewUnlockFC(t, nil)
	initialFcr := &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}
	feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, initialFcr))
	var attr fc.UnlockFeeCreditAttributes
	require.NoError(t, tx.UnmarshalAttributes(&attr))
	execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
	require.NoError(t, feeModule.validateUnlockFC(tx, &attr, execCtx))
	sm, err := feeModule.executeUnlockFC(tx, &attr, execCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	fcrUnit, err := feeModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, uint64(0), fcr.Locked)
	require.Equal(t, 50-sm.ActualFee, fcr.Balance) //-fee
	require.Equal(t, uint64(0), fcr.Timeout)
	// backlink is updated
	require.NotEqual(t, initialFcr.Backlink, fcr.Backlink)

}
