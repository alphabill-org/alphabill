package money

import (
	"fmt"
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

func TestModule_validateTransferTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const counter = 6
	const value = 100
	t.Run("ok", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateTransferTx(tx, attr, authProof, exeCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), fmt.Sprintf("transfer validation error: item %s does not exist: not found", unitID))
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: value, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: invalid unit data type")
	})
	t.Run("locked bill", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Locked: 1, Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: the bill is locked")
	})
	t.Run("invalid amount", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value + 1, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: the transaction value is not equal to the bill value")
	})
	t.Run("invalid counter - replay attack", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter - 1, OwnerPredicate: templates.AlwaysTrueBytes()}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: the transaction counter is not equal to the bill counter")
	})
	t.Run("owner error", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.NewP2pkh256BytesFromKey(pubKey)}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: evaluating owner predicate: executing predicate: failed to decode P2PKH256 signature: EOF")
	})
}

func TestModule_executeTransferTx(t *testing.T) {
	const counter = 6
	const value = 100
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	unitID := moneyid.NewBillID(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.NewP2pkh256BytesFromKey(pubKey), counter)
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, &money.BillData{Value: value, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
	exeCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(6))
	sm, err := module.executeTransferTx(tx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
	// read the state and make sure all that must be updated where updated
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.Owner(), attr.NewOwnerPredicate)
	require.EqualValues(t, bill.Value, value)
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.Locked, 0)
}
