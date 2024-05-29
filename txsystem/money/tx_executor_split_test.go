package money

import (
	gocrypto "crypto"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtx "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
)

func TestModule_validateSplitTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const counter = uint64(6)
	const billValue = uint64(100)
	t.Run("ok - 2-way split", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50, // - Amount split
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.NoError(t, module.validateSplitTx(tx, attr, exeCtx))
	})
	t.Run("ok - 3-way split", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			billValue-10-10, // two additional bills with value 10 are created
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.NoError(t, module.validateSplitTx(tx, attr, exeCtx))
	})
	t.Run("err - bill not found", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50,
			counter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "item 000000000000000000000000000000000000000000000000000000000000000200 does not exist: not found")
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: 6}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: invalid data type, unit is not of BillData type")
	})
	t.Run("err - bill locked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-50,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{Locked: 1, V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: bill is locked")
	})
	t.Run("err - invalid counter", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 20, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue-20,
			counter+1)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: the transaction counter is not equal to the unit counter")
	})
	t.Run("err - target units empty", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: target units are empty")
	})
	t.Run("err - target unit is nil", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{nil},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: target unit is nil at index 0")
	})
	t.Run("err - target unit amount is 0", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 0, OwnerCondition: templates.AlwaysTrueBytes()}},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: target unit amount is zero at index 0")
	})
	t.Run("err - target unit owner condition is empty", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{{Amount: 1, OwnerCondition: []byte{}}},
			billValue-1,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: target unit owner condition is empty at index 0")
	})
	t.Run("err - target unit amount overflow", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: math.MaxUint64, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 1, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			billValue,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: failed to add target unit amounts: uint64 sum overflow: [18446744073709551615 1]")
	})
	t.Run("err - remaining value is zero", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			0,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), "split error: remaining value is zero")
	})
	t.Run("err - amount plus remaining value is less than bill value", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			79,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx),
			"split error: the sum of the values to be transferred plus the remaining value must equal the value of the bill; sum=20 remainingValue=79 billValue=100")
	})
	t.Run("err - amount plus remaining value is less than bill value", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			81,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx),
			"split error: the sum of the values to be transferred plus the remaining value must equal the value of the bill; sum=20 remainingValue=81 billValue=100")
	})
	t.Run("owner predicate error", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createSplit(t, unitID, fcrID,
			[]*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			},
			80,
			counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysFalseBytes(), &money.BillData{V: billValue, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateSplitTx(tx, attr, exeCtx), `executing bearer predicate: predicate evaluated to "false"`)
	})
}

func TestModule_executeSplitTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const counter = uint64(6)
	const billValue = uint64(100)
	unitID := money.NewBillID(nil, []byte{2})
	tx, attr := createSplit(t, unitID, fcrID,
		[]*money.TargetUnit{
			{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
			{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
		},
		billValue-10-10, // two additional bills with value 10 are created
		counter)
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: billValue, Counter: counter}))
	exeCtx := testtx.NewMockExecutionContext(t, testtx.WithCurrentRound(6))
	sm, err := module.executeSplitTx(tx, attr, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	targets := []types.UnitID{tx.UnitID()}
	// 3 way split, so 3 targets
	sum := uint64(0)
	for i, targetUnit := range attr.TargetUnits {
		newUnitID := money.NewBillID(unitID, tx.HashForNewUnitID(gocrypto.SHA256, util.Uint32ToBytes(uint32(i))))
		targets = append(targets, newUnitID)
		// verify that the amount is correct
		u, err := module.state.GetUnit(newUnitID, false)
		require.NoError(t, err)
		// bill owner is changed to dust collector
		require.EqualValues(t, u.Bearer(), targetUnit.OwnerCondition)
		// bill value is now 0, the counter and "last round" are both updated
		bill, ok := u.Data().(*money.BillData)
		require.True(t, ok)
		require.EqualValues(t, bill.V, targetUnit.Amount)
		// newly created bill, so counter is 0
		require.EqualValues(t, bill.Counter, 0)
		require.EqualValues(t, bill.T, exeCtx.CurrentRound())
		require.EqualValues(t, bill.Locked, 0)
		sum += bill.V
	}
	require.EqualValues(t, sm.TargetUnits, targets)
	// target unit was also updated and it was credited correctly
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.EqualValues(t, u.Bearer(), templates.AlwaysTrueBytes())
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.V, billValue-sum)
	// newly created bill, so counter is 0
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, exeCtx.CurrentRound())
	require.EqualValues(t, bill.Locked, 0)
}
