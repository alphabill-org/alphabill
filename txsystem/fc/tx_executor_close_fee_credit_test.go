package fc

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestCloseFC_ValidateAndExecute(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	// create existing fee credit record for closeFC
	attr := testfc.NewCloseFCAttr()
	tx := testfc.NewCloseFC(t, attr)
	feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
	// execute closeFC transaction
	require.NoError(t, feeCreditModule.validateCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	sm, err := feeCreditModule.executeCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)
	// verify closeFC updated the FCR.Backlink
	fcrUnit, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, tx.Hash(crypto.SHA256), fcr.Backlink)
}

func TestFeeCredit_validateCloseFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}

	t.Run("Ok", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.NoError(t, feeModule.validateCloseFC(tx, &attr, execCtx))
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("UnitID has wrong type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithUnitID([]byte{8}))
		feeModule := newTestFeeModule(t, trustBase,
			withFeeCreditType([]byte{0xFF}),
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"fee credit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee authorization proof")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &testData{}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"fee credit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("Invalid amount", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: invalid amount: amount=51 fcr.Balance=50")
	})
	t.Run("Nil target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID(nil)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID([]byte{})))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("Close locked credit", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Locked: 1, Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: fee credit record is locked")
	})

}

type feeTestOption func(m *FeeCredit) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) feeTestOption {
	return func(m *FeeCredit) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func withFeeCreditType(feeType []byte) feeTestOption {
	return func(m *FeeCredit) error {
		m.feeCreditRecordUnitType = feeType
		return nil
	}
}

func newTestFeeModule(t *testing.T, tb map[string]abcrypto.Verifier, opts ...feeTestOption) *FeeCredit {
	m := &FeeCredit{
		hashAlgorithm:         crypto.SHA256,
		feeCalculator:         FixedFee(1),
		state:                 state.NewEmptyState(),
		systemIdentifier:      moneySystemID,
		moneySystemIdentifier: moneySystemID,
		trustBase:             tb,
	}
	for _, o := range opts {
		require.NoError(t, o(m))
	}
	return m
}
