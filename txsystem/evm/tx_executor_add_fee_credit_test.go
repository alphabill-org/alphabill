package evm

import (
	"crypto"
	"crypto/sha256"
	"math/big"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/evm"
	fcsdk "github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
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
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
		sm, err := feeCreditModule.executeAddFC(tx, &attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack - have to use error contains since backlink hash changes
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink= unit.backlink=")
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fcsdk.AddFeeCreditAttributes{FeeCreditTransfer: nil}))
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx proof is nil")
	})
	t.Run("transferFC server metadata is nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("actual target unit backlink")))),
						ServerMetadata:   nil,
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is missing server metadata")
	})
	t.Run("transferFC attributes unmarshal error", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("bill not transferred to fee credits of the target record", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithUnitID([]byte{1}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id: transferFC.TargetRecordId=0000000000000000000000000000000000000000000000000000000000000001FF tx.UnitId=01")
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(evm.DefaultSystemID))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
				testfc.WithFCOwnerCondition([]byte("wrong bearer")),
			))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fcsdk.FeeCreditRecord{}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"failed to extract public key from fee credit owner proof")
	})
	t.Run("invalid system id", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, nil, testtransaction.WithSystemID(0xFFFFFFFF)),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"addFC: invalid transferFC money system identifier FFFFFFFF (expected 00000001)")
	})
	t.Run("Invalid target systemID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(0xFFFFFFFF))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target system identifier: expected_target_system_id: 00000003 actual_target_system_id=FFFFFFFF")
	})
	t.Run("Invalid target recordID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t,
							testfc.NewTransferFCAttr(
								testfc.WithTargetSystemID(evm.DefaultSystemID),
								testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id: transferFC.TargetRecordId=6E6F7420657175616C20746F207472616E73616374696F6E2E756E69744964 tx.UnitId=0000000000000000000000000000000000000000000000000000000000000001FF")
	})
	t.Run("Invalid target unit backlink (fee credit record does not exist)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitBacklink([]byte("non-empty target unit backlink")))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink=6E6F6E2D656D7074792074617267657420756E6974206261636B6C696E6B unit.backlink=")
	})
	t.Run("Invalid target unit backlink (tx target unit backlink equals to fee credit record state hash and NOT backlink)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitBacklink([]byte("sent target unit backlink")))),
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
				Account:   &statedb.Account{Balance: big.NewInt(100)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink=73656E742074617267657420756E6974206261636B6C696E6B unit.backlink=61637475616C2074617267657420756E6974206261636B6C696E6B")
	})
	t.Run("ok target unit backlink (tx target unit backlink equals fee credit record)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitBacklink([]byte("actual target unit backlink")))),
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
				Account:   &statedb.Account{Balance: big.NewInt(100)},
				AlphaBill: &statedb.AlphaBillLink{TxHash: []byte("actual target unit backlink")},
			}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("EarliestAdditionTime in the future NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithEarliestAdditionTime(11))),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx), "invalid transferFC timeout: earliest=11 latest=10 current=10")
	})
	t.Run("EarliestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithEarliestAdditionTime(10))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithLatestAdditionTime(9))),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC timeout: earliest=0 latest=9 current=10")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithAmount(100))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 90}),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fcsdk.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithAmount(100))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)
		require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
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
	signer, ver := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": ver}
	pubKeyBytes, err := ver.MarshalPublicKey()
	require.NoError(t, err)
	pubHash := hash.Sum256(pubKeyBytes)
	privKeyHash := hashOfPrivateKey(t, signer)
	feeCreditModule := newTestFeeModule(t, trustBase)
	addFeeOrder := newAddFCTx(t,
		privKeyHash,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKeyHash(pubHash)),
			testfc.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100), testfc.WithTargetRecordID(privKeyHash), testfc.WithTargetSystemID(evm.DefaultSystemID)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	attr := new(fcsdk.AddFeeCreditAttributes)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))

	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5}))
	metaData, err := feeCreditModule.executeAddFC(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	require.NotNil(t, metaData)
	require.EqualValues(t, evmTestFeeCalculator(), metaData.ActualFee)
	// validate stateDB
	stateDB := statedb.NewStateDB(feeCreditModule.state, logger.New(t))
	addr, err := generateAddress(pubKeyBytes)
	require.NoError(t, err)
	balance := stateDB.GetBalance(addr)
	// balance is equal to 100 - "transfer fee" - "add fee" to wei
	remainingCredit := new(big.Int).Sub(alphaToWei(100), alphaToWei(transferFcFee))
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
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
			testfc.WithTransferFCTx(
				&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithAmount(10),
							testfc.WithTargetRecordID(privKeyHash),
							testfc.WithTargetSystemID(evm.DefaultSystemID),
							testfc.WithTargetUnitBacklink(abData.TxHash)),
						testtransaction.WithSystemID(0x00000001), testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKeyHash(pubHash))),
					ServerMetadata: &types.ServerMetadata{ActualFee: transferFcFee},
				})),
		signer, 7)
	require.NoError(t, addFeeOrder.UnmarshalAttributes(attr))
	require.NoError(t, feeCreditModule.validateAddFC(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5}))
	metaData, err = feeCreditModule.executeAddFC(addFeeOrder, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 5})
	require.NoError(t, err)
	require.NotNil(t, metaData)
	remainingCredit = new(big.Int).Add(remainingCredit, alphaToWei(10))
	balance = stateDB.GetBalance(addr)
	// balance is equal to remaining+10-"transfer fee 1" -"ass fee = 2" to wei
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(transferFcFee))
	remainingCredit = new(big.Int).Sub(remainingCredit, alphaToWei(evmTestFeeCalculator()))
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

func newTestFeeModule(t *testing.T, tb map[string]abcrypto.Verifier, opts ...feeTestOption) *FeeAccount {
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
