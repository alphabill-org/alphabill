package money

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtx "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestModule_validateReclaimFCTx(t *testing.T) {
	const (
		amount  = uint64(100)
		counter = uint64(4)
	)
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	t.Run("Ok", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.NoError(t, module.validateReclaimFCTx(tx, attr, exeCtx))
	})
	t.Run("Bill is missing", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier)
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx),
			"get unit error: item 000000000000000000000000000000000000000000000000000000000000000000 does not exist: not found")
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		exeCtx := testtx.NewMockExecutionContext(t)
		module := newTestMoneyModule(t, verifier, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 10}))
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid unit type")
	})
	t.Run("Fee credit record exists in tx", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "fee tx cannot contain fee credit reference")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithFeeProof([]byte{0, 0, 0, 0}))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "fee tx cannot contain fee authorization proof")
	})
	t.Run("Invalid target unit", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil,
			testtransaction.WithUnitID(money.NewFeeCreditRecordID(nil, []byte{2})))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid target unit")
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer,
			testutils.NewReclaimFCAttr(t, signer,
				testutils.WithReclaimFCClosureTx(
					&types.TransactionRecord{
						TransactionOrder: testutils.NewCloseFC(t, signer,
							testutils.NewCloseFCAttr(
								testutils.WithCloseFCAmount(2),
								testutils.WithCloseFCTargetUnitCounter(counter),
							),
						),
						ServerMetadata: &types.ServerMetadata{ActualFee: 10},
					},
				),
			),
		)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "the transaction fees cannot exceed the transferred value")
	})
	t.Run("Invalid target unit counter", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter + 1}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid target unit counter")
	})
	t.Run("Invalid counter", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer, testutils.WithReclaimFCCounter(counter+1)))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "the transaction counter is not equal to the unit counter")
	})
	t.Run("owner error", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, nil)
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysFalseBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), `predicate evaluated to "false"`)
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer,
			testutils.WithReclaimFCClosureProof(newInvalidProof(t, signer))))
		attr := &fcsdk.ReclaimFeeCreditAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(attr))
		module := newTestMoneyModule(t, verifier,
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateReclaimFCTx(tx, attr, exeCtx), "invalid proof: proof block hash does not match to block hash in unicity certificate")
	})
}

func TestModule_executeReclaimFCTx(t *testing.T) {
	const (
		amount  = uint64(100)
		counter = uint64(4)
	)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	tx := testutils.NewReclaimFC(t, signer, nil)
	attr := &fcsdk.ReclaimFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	module := newTestMoneyModule(t, verifier,
		withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &money.BillData{V: amount, Counter: counter}))
	exeCtx := testtx.NewMockExecutionContext(t)
	sm, err := module.executeReclaimFCTx(tx, attr, exeCtx)
	require.NoError(t, err)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{tx.UnitID()}, sm.TargetUnits)
	// verify changes
	u, err := module.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	require.EqualValues(t, u.Bearer(), templates.AlwaysTrueBytes())
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	// target bill is credited correct amount (using default values from testutils)
	v := 50 - 10 - module.feeCalculator()
	require.EqualValues(t, bill.V, amount+v)
	// counter is incremented
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, exeCtx.CurrentRound())
	require.EqualValues(t, bill.Locked, 0)
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	attr := testutils.NewDefaultReclaimFCAttr(t, signer)
	attr.CloseFeeCreditProof.BlockHeaderHash = []byte("invalid hash")
	return attr.CloseFeeCreditProof
}
