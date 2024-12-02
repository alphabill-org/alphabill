package evm

import (
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/stretchr/testify/require"
)

type testData struct {
	_ struct{} `cbor:",toarray"`
}

func (t *testData) Write(hasher abhash.Hasher) { hasher.Write(t) }
func (t *testData) SummaryValueInput() uint64 {
	return 0
}
func (t *testData) Copy() types.UnitData { return &testData{} }
func (t *testData) Owner() []byte        { return nil }

func TestFeeCredit_validateCloseFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	address, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)

	t.Run("Ok", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCCounter(10)))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 10},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.NoError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx))
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1, 2, 3}}),
		)
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 10},
			}))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee credit reference")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithFeeProof([]byte{1}))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 10},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"invalid fee credit transaction: fee transaction cannot contain fee authorization proof")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil)
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(address.Bytes(), nil, &testData{}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"invalid unit type: not evm object")
	})
	t.Run("Invalid amount", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51)))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 10},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: invalid amount: amount=51 fcr.Balance=50")
	})
	t.Run("Nil target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCCounter(10), testfc.WithCloseFCTargetUnitID(nil)))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 10},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID([]byte{})))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		authProof := fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
		require.NoError(t, tx.SetAuthProof(authProof))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := testctx.NewMockExecutionContext(testctx.WithCurrentRound(5))
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, &authProof, execCtx),
			"not enough funds: max fee cannot exceed fee credit record balance: tx.maxFee=51 fcr.Balance=50")
	})
}

func TestCloseFC_ValidateAndExecute(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	address, err := generateAddress(pubKey)
	require.NoError(t, err)

	// create existing fee credit record for closeFC
	attr := testfc.NewCloseFCAttr(testfc.WithCloseFCCounter(10))
	tx := testfc.NewCloseFC(t, signer, attr, testtransaction.WithUnitID(address.Bytes()))
	authProof := &fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, tx, signer)}
	require.NoError(t, tx.SetAuthProof(authProof))
	feeModule := newTestFeeModule(t, trustBase,
		withStateUnit(address.Bytes(), nil, &statedb.StateObject{
			Address:   address,
			Account:   &statedb.Account{Balance: alphaToWei(50)},
			AlphaBill: &statedb.AlphaBillLink{Counter: 10, OwnerPredicate: templates.AlwaysTrueBytes()},
		})) // execute closeFC transaction
	require.NoError(t, feeModule.validateCloseFC(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10))))
	sm, err := feeModule.executeCloseFC(tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	// verify closeFC updated the FCR.Backlink
	fcrUnit, err := feeModule.state.GetUnit(address.Bytes(), false)
	require.NoError(t, err)
	obj, ok := fcrUnit.Data().(*statedb.StateObject)
	require.True(t, ok)
	require.EqualValues(t, 11, obj.AlphaBill.Counter)
	require.EqualValues(t, 0, obj.Account.Balance.Uint64())
	require.EqualValues(t, templates.AlwaysTrueBytes(), obj.AlphaBill.OwnerPredicate)
}
