package evm

import (
	"crypto"
	"hash"
	"testing"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/stretchr/testify/require"
)

type testData struct {
	_ struct{} `cbor:",toarray"`
}

func (t *testData) Write(hasher hash.Hash) error { return nil }
func (t *testData) SummaryValueInput() uint64 {
	return 0
}
func (t *testData) Copy() types.UnitData { return &testData{} }
func (t *testData) IncrementCounter()    {}

func TestFeeCredit_validateCloseFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	address, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)

	t.Run("Ok", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.NoError(t, feeModule.validateCloseFC(tx, &attr, execCtx))
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1, 2, 3}}),
		)
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("Fee proof exists", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithFeeProof([]byte{1}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee authorization proof")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase, withStateUnit(address.Bytes(), nil, &testData{}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"invalid unit type: not evm object")
	})
	t.Run("Invalid amount", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51)))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: invalid amount: amount=51 fcr.Balance=50")
	})
	t.Run("Nil target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID(nil)))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCTargetUnitID([]byte{})))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
			"validation error: TargetUnitID is empty")
	})
	t.Run("Empty target unit id", func(t *testing.T) {
		tx := testfc.NewCloseFC(t, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: alphaToWei(50)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		var attr fcsdk.CloseFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		require.EqualError(t, feeModule.validateCloseFC(tx, &attr, execCtx),
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
	attr := testfc.NewCloseFCAttr()
	tx := testfc.NewCloseFC(t, attr, testtransaction.WithUnitID(address.Bytes()))
	require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	feeModule := newTestFeeModule(t, trustBase,
		withStateUnit(address.Bytes(), nil, &statedb.StateObject{
			Address:   address,
			Account:   &statedb.Account{Balance: alphaToWei(50)},
			AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
		})) // execute closeFC transaction
	require.NoError(t, feeModule.validateCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	sm, err := feeModule.executeCloseFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)
	// verify closeFC updated the FCR.Backlink
	fcrUnit, err := feeModule.state.GetUnit(address.Bytes(), false)
	require.NoError(t, err)
	obj, ok := fcrUnit.Data().(*statedb.StateObject)
	require.True(t, ok)
	require.Equal(t, tx.Hash(crypto.SHA256), obj.AlphaBill.TxHash)
	require.EqualValues(t, 0, obj.Account.Balance.Int64())
}
