package money

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestModule_validateTransferDCTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const counter = 6
	const targetCounter = 2
	const value = 100
	var targetUnitID = test.RandomBytes(32)
	t.Run("ok", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx), fmt.Sprintf("validateTransferDC error: item %s does not exist: not found", unitID))
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: value, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx), "validateTransferDC error: invalid unit data type")
	})
	t.Run("bill is locked", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value + 1, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx), "validateTransferDC error: the transaction value is not equal to the bill value")
	})
	t.Run("invalid counter - replay attack", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter-1, targetUnitID, targetCounter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx), "validateTransferDC error: the transaction counter is not equal to the bill counter")
	})
	t.Run("owner error", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, targetUnitID, targetCounter)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferDCTx(tx, attr, authProof, exeCtx), "validateTransferDC error: evaluating owner predicate: executing predicate: failed to decode P2PKH256 signature: EOF")
	})
}

func TestModule_executeTransferDCTx(t *testing.T) {
	const counter = 6
	const value = 100
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	unitID := moneyid.NewBillID(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	tx, attr, authProof := createDCTransfer(t, unitID, fcrID, value, counter, test.RandomBytes(32), 4)
	module := newTestMoneyModule(t, verifier,
		withStateUnit(DustCollectorMoneySupplyID, money.NewBillData(1000, DustCollectorPredicate)),
		withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
	exeCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(6))
	// get dust bill value before
	d, err := module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	dustBill, ok := d.Data().(*money.BillData)
	require.True(t, ok)
	dustBefore := dustBill.Copy()
	sm, err := module.executeTransferDCTx(tx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID, DustCollectorMoneySupplyID}, sm.TargetUnits)
	// read the state and make sure all that must be updated where updated
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	// bill owner is changed to dust collector
	require.EqualValues(t, dustBill.Owner(), DustCollectorPredicate)
	// bill value is now 0, the counter and "last round" are both updated
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.Value, 0)
	// bill value has been added to the dust collector
	d, err = module.state.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	dustBill, ok = d.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, dustBill.Value, dustBefore.(*money.BillData).Value+value)
}
