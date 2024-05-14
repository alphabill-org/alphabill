package money

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtx "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/stretchr/testify/require"
)

func TestModule_validateTransferDCTx(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	const counter = 6
	const targetCounter = 2
	const value = 100
	var targetUnitID = test.RandomBytes(32)
	t.Run("ok", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.NoError(t, module.validateTransferDCTx(tx, attr, exeCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "item 000000000000000000000000000000000000000000000000000000000000000200 does not exist: not found")
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: value}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "validateTransferDC error: invalid data type")
	})
	t.Run("bill is locked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{Locked: 1, V: value, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "validateTransferDC error: bill is locked")
	})
	t.Run("bill is locked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value + 1, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "validateTransferDC error: transaction value must be equal to bill value")
	})
	t.Run("invalid counter - replay attack", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter-1, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "validateTransferDC error: the transaction counter is not equal to the unit counter")
	})
	t.Run("owner error", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr := createDCTransfer(t, unitID, value, counter, targetUnitID, targetCounter)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: value, Counter: counter}))
		exeCtx := testtx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateTransferDCTx(tx, attr, exeCtx), "validateTransferDC error: executing predicate: failed to decode P2PKH256 signature: EOF")
	})
}

func TestModule_executeTransferDCTx(t *testing.T) {
	const counter = 6
	const value = 100
	_, verifier := testsig.CreateSignerAndVerifier(t)
	unitID := money.NewBillID(nil, []byte{2})
	tx, attr := createDCTransfer(t, unitID, value, counter, test.RandomBytes(32), 4)
	module := newTestMoneyModule(t, verifier,
		withStateUnit(DustCollectorMoneySupplyID, DustCollectorPredicate, &money.BillData{V: 1000, T: 0, Counter: 0}),
		withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
	exeCtx := testtx.NewMockExecutionContext(t, testtx.WithCurrentRound(6))
	// get dust bill value before
	d, err := module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	dustBill, ok := d.Data().(*money.BillData)
	require.True(t, ok)
	dustBefore := dustBill.Copy()
	sm, err := module.executeTransferDCTx(tx, attr, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID, DustCollectorMoneySupplyID}, sm.TargetUnits)
	// read the state and make sure all that must be updated where updated
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	// bill owner is changed to dust collector
	require.EqualValues(t, u.Bearer(), DustCollectorPredicate)
	// bill value is now 0, the counter and "last round" are both updated
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.V, 0)
	require.EqualValues(t, bill.T, 6)
	require.EqualValues(t, bill.Locked, 0)
	// bill value has been added to the dust collector
	d, err = module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	dustBill, ok = d.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, dustBill.V, dustBefore.(*money.BillData).V+value)
	require.EqualValues(t, dustBill.Counter, dustBefore.(*money.BillData).Counter+1)
	// T and Locked have not been changed
	require.EqualValues(t, dustBill.Locked, dustBefore.(*money.BillData).Locked)
	require.EqualValues(t, dustBill.T, dustBefore.(*money.BillData).T)
}
