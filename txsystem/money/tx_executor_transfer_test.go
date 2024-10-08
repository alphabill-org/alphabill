package money

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestModule_validateTransferTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	const counter = 6
	const value = 100
	t.Run("ok", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateTransferTx(tx, attr, authProof, exeCtx))
	})
	t.Run("unit does not exist", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier)
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: item 000000000000000000000000000000000000000000000000000000000000000201 does not exist: not found")
	})
	t.Run("unit is not bill data", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{Balance: value}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: invalid data type")
	})
	t.Run("locked bill", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{Locked: 1, V: value, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: bill is locked")
	})
	t.Run("invalid amount", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value + 1, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: transaction value must be equal to bill value")
	})
	t.Run("invalid counter - replay attack", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter - 1}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "transfer validation error: the transaction counter is not equal to the unit counter")
	})
	t.Run("owner error", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.AlwaysTrueBytes(), counter)
		module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{V: value, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		require.EqualError(t, module.validateTransferTx(tx, attr, authProof, exeCtx), "evaluating owner predicate: executing predicate: failed to decode P2PKH256 signature: EOF")
	})
}

func TestModule_executeTransferTx(t *testing.T) {
	const counter = 6
	const value = 100
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	unitID := money.NewBillID(nil, []byte{2})
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	tx, attr, authProof := createBillTransfer(t, unitID, fcrID, value, templates.NewP2pkh256BytesFromKey(pubKey), counter)
	module := newTestMoneyModule(t, verifier, withStateUnit(unitID, templates.AlwaysTrueBytes(), &money.BillData{V: value, Counter: counter}))
	exeCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(6))
	sm, err := module.executeTransferTx(tx, attr, authProof, exeCtx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
	// read the state and make sure all that must be updated where updated
	u, err := module.state.GetUnit(unitID, false)
	require.NoError(t, err)
	require.EqualValues(t, u.Owner(), attr.NewOwnerPredicate)
	bill, ok := u.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, bill.V, value)
	require.EqualValues(t, bill.Counter, counter+1)
	require.EqualValues(t, bill.T, 6)
	require.EqualValues(t, bill.Locked, 0)
}
