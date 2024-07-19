package evm

import (
	"crypto"
	"crypto/sha256"
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func hashOfPrivateKey(t *testing.T, signer abcrypto.Signer) []byte {
	t.Helper()
	privKeyBytes, err := signer.MarshalPrivateKey()
	require.NoError(t, err)
	h := sha256.Sum256(privKeyBytes)
	return h[:]
}

func newTxPayload(t *testing.T, txType string, unitID []byte, timeout uint64, fcrID []byte, attr interface{}) *types.Payload {
	attrBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)
	return &types.Payload{
		SystemID:   evm.DefaultSystemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: 1,
			FeeCreditRecordID: fcrID,
		},
	}
}

func newAddFCTx(t *testing.T, unitID []byte, attr *fcsdk.AddFeeCreditAttributes, signer abcrypto.Signer, timeout uint64) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Payload: newTxPayload(t, fcsdk.PayloadTypeAddFeeCredit, unitID, timeout, nil, attr),
	}
	require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return tx
}

func evmTestFeeCalculator() uint64 {
	return 1
}

func TestAddFC_ValidateAddNewFeeCreditTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	fcrID := testfc.NewFeeCreditRecordID(t, signer)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
		sm, err := feeCreditModule.executeAddFC(tx, &attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=<nil> unit.counter=0")
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fcsdk.AddFeeCreditAttributes{FeeCreditTransfer: nil}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx record is nil")
	})
	t.Run("transferFC tx order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fcsdk.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is nil")
	})
	t.Run("transferFC proof is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fcsdk.AddFeeCreditAttributes{
				FeeCreditTransfer: &types.TransactionRecord{
					TransactionOrder: &types.TransactionOrder{},
				},
				FeeCreditTransferProof: nil,
			},
		))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx proof is nil")
	})
	t.Run("transferFC server metadata is nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(4))),
						ServerMetadata:   nil,
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is missing server metadata")
	})
	t.Run("transferFC attributes unmarshal error", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewAddFC(t, signer, nil,
							testtransaction.WithPayloadType(fcsdk.PayloadTypeTransferFeeCredit)),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"failed to unmarshal transfer fee credit attributes: cbor: cannot unmarshal array into Go value of type fc.TransferFeeCreditAttributes (cannot decode CBOR array to struct with different number of elements)")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1, 2, 3}}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("bill not transferred to fee credits of the target record", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithUnitID([]byte{1}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			fmt.Sprintf("invalid transferFC target record id: transferFC.TargetRecordId=%s tx.UnitId=01", fcrID))
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
				testfc.WithFCOwnerCondition([]byte("wrong bearer")),
			))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"failed to extract public key from fee credit owner proof")
	})
	t.Run("invalid system id", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, nil, testtransaction.WithSystemID(0xFFFFFFFF)),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC money system identifier FFFFFFFF (expected 00000001)")
	})
	t.Run("Invalid target systemID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetSystemID(0xFFFFFFFF))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target system identifier: expected_target_system_id: 00000003 actual_target_system_id=FFFFFFFF")
	})
	t.Run("Invalid target recordID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer,
							testfc.NewTransferFCAttr(t, signer,
								testfc.WithTargetSystemID(evm.DefaultSystemID),
								testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			fmt.Sprintf("invalid transferFC target record id: transferFC.TargetRecordId=6E6F7420657175616C20746F207472616E73616374696F6E2E756E69744964 tx.UnitId=%s", fcrID))
	})
	t.Run("invalid target unit counter (fee credit record does not exist)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitCounter(4))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=4 unit.counter=<nil>")
	})
	t.Run("invalid target unit counter (tx.targetUnitCounter < unit.counter)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitCounter(4))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit counter: transferFC.targetUnitCounter=4 unit.counter=<nil>")
	})
	t.Run("ok target unit counter (tx target unit counter equals fee credit record counter)", func(t *testing.T) {
		t.SkipNow() // TODO fix EVM fee credit records
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitCounter(4)),
							testtransaction.WithUnitID(fcrID),
						),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		address, err := generateAddress(pubKey)
		require.NoError(t, err)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(address.Bytes(), nil, &statedb.StateObject{
				Address:   address,
				Account:   &statedb.Account{Balance: uint256.NewInt(100)},
				AlphaBill: &statedb.AlphaBillLink{Counter: 4},
			}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithLatestAdditionTime(9))),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC timeout: latestAdditionTime=9 currentRoundNumber=10")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,

							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithAmount(100))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 90}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,

							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithAmount(100))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,

							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithAmount(100))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
					},
				),
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"proof is not valid: proof block hash does not match to block hash in unicity certificate")
	})
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	addFC := testfc.NewAddFC(t, signer, nil)
	attr := &fcsdk.AddFeeCreditAttributes{}
	require.NoError(t, addFC.UnmarshalAttributes(attr))

	attr.FeeCreditTransferProof.BlockHeaderHash = []byte("invalid hash")
	return attr.FeeCreditTransferProof
}

func Test_addFeeCreditTxAndUpdate(t *testing.T) {
	const transferFcFee = 1
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := hash.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	feeCreditModule := newTestFeeModule(t, trustBase)
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCRecord(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(evm.DefaultSystemID)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee, SuccessIndicator: types.TxStatusSuccessful},
				})),
		signer, 7)
	attr := new(fcsdk.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))

	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))))
	metaData, err := feeCreditModule.executeAddFC(addFeeOrder, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5)))
	require.NoError(t, err)
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	// validate stateDB
	stateDB := statedb.NewStateDB(feeCreditModule.state, logger.New(t))
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	// balance is equal to 100 - "transfer fee" - "add fee" to wei
	remainingCredit := new(uint256.Int).Sub(alphaToWei(100), alphaToWei(transferFcFee))
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner condition set
	u, err := feeCreditModule.state.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Bearer())

	abData := stateDB.GetAlphaBillData(addr)
	// add more funds
	addFeeOrder = newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCRecord(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, signer,
						testfc.NewTransferFCAttr(t, signer, testfc.WithAmount(10),
							testfc.WithTargetRecordID(privKeyHash),
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitCounter(abData.Counter)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee, SuccessIndicator: types.TxStatusSuccessful},
				})),
		signer, 7)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))))
	metaData, err = feeCreditModule.executeAddFC(addFeeOrder, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5)))
	require.NoError(t, err)
	require.NotNil(t, metaData)
	remainingCredit = new(uint256.Int).Add(remainingCredit, alphaToWei(10))
	balance = stateDB.GetBalance(addr)
	// balance is equal to remaining+10-"transfer fee 1" -"ass fee = 2" to wei
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(transferFcFee))
	remainingCredit = new(uint256.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
	require.EqualValues(t, balance, remainingCredit)
	// check owner condition
	u, err = feeCreditModule.state.GetUnit(addr.Bytes(), false)
	require.NoError(t, err)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(pubHash), u.Bearer())
}

type feeTestOption func(m *FeeAccount) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) feeTestOption {
	return func(m *FeeAccount) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func newTestFeeModule(t *testing.T, tb types.RootTrustBase, opts ...feeTestOption) *FeeAccount {
	m := &FeeAccount{
		hashAlgorithm: crypto.SHA256,
		feeCalculator: FixedFee(1),
		state:         state.NewEmptyState(),
		systemID:      evm.DefaultSystemID,
		moneySystemID: money.DefaultSystemID,
		trustBase:     tb,
	}
	for _, o := range opts {
		require.NoError(t, o(m))
	}
	return m
}
