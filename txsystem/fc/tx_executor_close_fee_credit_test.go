package fc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/stretchr/testify/require"
)

func TestCloseFC_ValidateAndExecute(t *testing.T) {
	targetPDR := moneyid.PDR()
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	// create existing fee credit record for closeFC
	attr := testfc.NewCloseFCAttr()
	authProof := &fc.CloseFeeCreditAuthProof{OwnerProof: templates.EmptyArgument()}
	tx := testfc.NewCloseFC(t, signer, attr, testtransaction.WithAuthProof(authProof))
	feeCreditModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
	// execute closeFC transaction
	require.NoError(t, feeCreditModule.validateCloseFC(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))))
	sm, err := feeCreditModule.executeCloseFC(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	// verify closeFC updated the FCR.Counter
	fcrUnit, err := feeCreditModule.state.GetUnit(tx.UnitID, false)
	require.NoError(t, err)
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.Equal(t, uint64(1), fcr.Counter)
}

func TestFeeCredit_validateCloseFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	targetPDR := moneyid.PDR()

	t.Run("Ok", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.NoError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx))
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee credit reference")
	})
	t.Run("UnitID has wrong type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithUnitID([]byte{8}))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase,
			withFeeCreditType(0xFF),
			withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"fee credit error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithFeeProof(feeProof))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee authorization proof")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &testData{}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"fee credit error: invalid unit type: unit is not fee credit record")
	})
	t.Run("Invalid amount", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51)))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: invalid amount: amount=51 fcr.Balance=50")
	})
	t.Run("Invalid counter", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(50), testfc.WithCloseFCCounter(10)))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Counter: 11, Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: invalid counter: counter=10 fcr.Counter=11")
	})
	t.Run("Nil target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID(nil)))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID([]byte{})))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
	t.Run("Close locked credit", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		feeModule := newTestFeeModule(t, &targetPDR, trustBase, withStateUnit(tx.UnitID, &fc.FeeCreditRecord{Locked: 1, Balance: 50}))
		var attr fc.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		var authProof fc.CloseFeeCreditAuthProof
		require.NoError(t, tx.UnmarshalAuthProof(&authProof))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: fee credit record is locked")
	})

}

type feeTestOption func(m *FeeCreditModule) error

func withStateUnit(unitID []byte, data types.UnitData) feeTestOption {
	return func(m *FeeCreditModule) error {
		return m.state.Apply(state.AddUnit(unitID, data))
	}
}

func withFeeCreditType(feeType uint32) feeTestOption {
	return func(m *FeeCreditModule) error {
		m.feeCreditRecordUnitType = feeType
		return nil
	}
}

func withFeePredicateRunner(r predicates.PredicateRunner) feeTestOption {
	return func(m *FeeCreditModule) error {
		m.execPredicate = r
		return nil
	}
}

func newTestFeeModule(t *testing.T, pdr *types.PartitionDescriptionRecord, tb types.RootTrustBase, opts ...feeTestOption) *FeeCreditModule {
	m := &FeeCreditModule{
		hashAlgorithm:    crypto.SHA256,
		state:            state.NewEmptyState(),
		pdr:              *pdr,
		moneyPartitionID: moneyPartitionID,
		trustBase:        tb,
		execPredicate: func(predicate types.PredicateBytes, args []byte, sigBytesFn func() ([]byte, error), env predicates.TxContext) error {
			return nil
		},
		feeCreditRecordUnitType: money.FeeCreditRecordUnitType,
	}
	for _, o := range opts {
		require.NoError(t, o(m))
	}
	return m
}
