package fc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestCloseFC_ValidateAndExecute(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	// create existing fee credit record for closeFC
	attr := testfc.NewCloseFCAttr()
	tx := testfc.NewCloseFC(t, signer, attr)
	feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
	// execute closeFC transaction
	require.NoError(t, feeCreditModule.validateCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}))
	sm, err := feeCreditModule.executeCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)
	// verify closeFC updated the FCR.Counter
	fcrUnit, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, uint64(1), fcr.Counter)
}

func TestFeeCredit_validateCloseFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)

	t.Run("Ok", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.NoError(t, feeModule.validateCloseFC(tx, &attr, execCtx))
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("UnitID has wrong type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithUnitID([]byte{8}))
		feeModule := newTestFeeModule(t, trustBase,
			withFeeCreditType([]byte{0xFF}),
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"fee credit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee authorization proof")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &testData{}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"fee credit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("Invalid amount", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: invalid amount: amount=51 fcr.Balance=50")
	})
	t.Run("Invalid counter", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(50), testfc.WithCloseFCCounter(10)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Counter: 11, Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 10}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: invalid counter: counter=10 fcr.Counter=11")
	})
	t.Run("Nil target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID(nil)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID([]byte{})))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("Close locked credit", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Locked: 1, Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
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

func newTestFeeModule(t *testing.T, tb types.RootTrustBase, opts ...feeTestOption) *FeeCredit {
	m := &FeeCredit{
		hashAlgorithm:         crypto.SHA256,
		feeCalculator:         FixedFee(1),
		state:                 state.NewEmptyState(),
		systemIdentifier:      moneySystemID,
		moneySystemIdentifier: moneySystemID,
		trustBase:             tb,
		execPredicate: func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error {
			return nil
		},
		feeCreditRecordUnitType: money.FeeCreditRecordUnitType,
	}
	for _, o := range opts {
		require.NoError(t, o(m))
	}
	return m
}
