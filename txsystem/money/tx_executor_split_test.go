package money

import (
	"fmt"
	"math"
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

func TestModule_validateSplitTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	authProof := &money.SplitAuthProof{OwnerProof: nil}
	const counter = uint64(6)
	const billValue = uint64(100)
	t.Run("ok - 2-way split", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateSplitTx(tx, attr, authProof, exeCtx))
	})
	t.Run("ok - 3-way split", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
			{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
			{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
		}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateSplitTx(tx, attr, authProof, exeCtx))
	})
	t.Run("err - bill not found", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), fmt.Sprintf("item %s does not exist: not found", unitID))
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 6, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: invalid data type, unit is not of BillData type")
	})
	t.Run("err - invalid counter", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 20, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter+1)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: the transaction counter is not equal to the unit counter")
	})
	t.Run("err - target units empty", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: target units are empty")
	})
	t.Run("err - target unit is nil", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{nil}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: target unit is nil at index 0")
	})
	t.Run("err - target unit amount is 0", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 0, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: target unit amount is zero at index 0")
	})
	t.Run("err - target unit owner predicate is empty", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{{Amount: 1, OwnerPredicate: []byte{}}}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: target unit owner predicate is empty at index 0")
	})
	t.Run("err - target unit amount overflow", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
			{Amount: math.MaxUint64, OwnerPredicate: templates.AlwaysTrueBytes()},
			{Amount: 1, OwnerPredicate: templates.AlwaysTrueBytes()},
		}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), "split error: failed to add target unit amounts: uint64 sum overflow: [18446744073709551615 1]")
	})
	t.Run("err - sum exceeds bill value", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
			{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()},
			{Amount: 51, OwnerPredicate: templates.AlwaysTrueBytes()},
		}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx),
			"split error: the sum of the values to be transferred must be less than the value of the bill; sum=101 billValue=100")
	})
	t.Run("err - sum equals bill value", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
			{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()},
			{Amount: 50, OwnerPredicate: templates.AlwaysTrueBytes()},
		}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx),
			"split error: the sum of the values to be transferred must be less than the value of the bill; sum=100 billValue=100")
	})
	t.Run("owner predicate error", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
			{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
			{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
		}, counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateSplitTx(tx, attr, authProof, exeCtx), `evaluating owner predicate: predicate evaluated to "false"`)
	})
}

func TestModule_executeSplitTx(t *testing.T) {
	pdr := moneyid.PDR()
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	authProof := &money.SplitAuthProof{OwnerProof: nil}
	const counter = uint64(6)
	const billValue = uint64(100)
	unitID, err := pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, moneyid.Random)
	require.NoError(t, err)
	tx, attr, _ := createSplit(t, unitID, fcrID, []*money.TargetUnit{
		{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
		{Amount: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
	}, counter)
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: billValue, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
	exeCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(6))
	sm, err := module.executeSplitTx(tx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	targets := []types.UnitID{tx.UnitID}
	// 3 way split, so 3 targets
	idGen := money.PrndSh(tx)
	sum := uint64(0)
	for _, targetUnit := range attr.TargetUnits {
		newUnitID, err := pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, idGen)
		require.NoError(t, err)
		targets = append(targets, newUnitID)
		// verify that the amount is correct
		u, err := module.state.GetUnit(newUnitID, false)
		require.NoError(t, err)
		// bill value is now 0, the counter and "last round" are both updated
		bill, ok := u.Data().(*money.BillData)
		require.True(t, ok)
		require.EqualValues(t, bill.Value, targetUnit.Amount)
		// bill owner is changed to dust collector
		require.EqualValues(t, bill.Owner(), targetUnit.OwnerPredicate)
		// newly created bill, so counter is 0
		require.EqualValues(t, bill.Counter, 0)
		sum += bill.Value
	}
	require.EqualValues(t, sm.TargetUnits, targets)
	// target unit was also updated and it was credited correctly
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Value, billValue-sum)
	require.EqualValues(t, bill.Owner(), templates.AlwaysTrueBytes())
	// newly created bill, so counter is 0
	require.EqualValues(t, bill.Counter, counter+1)
}
