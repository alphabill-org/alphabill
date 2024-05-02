package fc

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"

	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
)

func TestAddFC_ValidateAddNewFeeCreditTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
		sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack - have to use error contains since backlink hash changes
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink= unit.backlink=")
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fc.AddFeeCreditAttributes{FeeCreditTransfer: nil}))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx record is nil")
	})
	t.Run("transferFC tx order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}}))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is nil")
	})
	t.Run("transferFC proof is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fc.AddFeeCreditAttributes{
				FeeCreditTransfer: &types.TransactionRecord{
					TransactionOrder: &types.TransactionOrder{},
				},
				FeeCreditTransferProof: nil,
			},
		))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
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
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is missing server metadata")
	})
	t.Run("transferFC type not valid", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewAddFC(t, signer, nil),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transfer fee credit transaction payload type: addFC")
	})
	t.Run("transferFC attributes unmarshal error", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewAddFC(t, signer, nil,
							testtransaction.WithPayloadType(fc.PayloadTypeTransferFeeCredit)),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transfer tx attributes error: failed to unmarshal transfer payload: cbor: cannot unmarshal array into Go value of type fc.TransferFeeCreditAttributes (cannot decode CBOR array to struct with different number of elements)")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("bill not transferred to fee credits of the target record", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithUnitID([]byte{1}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id: transferFC.TargetRecordId=0000000000000000000000000000000000000000000000000000000000000001FF tx.UnitId=01")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil, testtransaction.WithUnitID([]byte{1}))
		feeCreditModule := newTestFeeModule(t, trustBase, withFeeCreditType([]byte{0xff}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"get frc error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithFCOwnerCondition([]byte("wrong bearer")),
			))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid owner condition: expected=83004101F6 actual=77726F6E6720626561726572")
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
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
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
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target system identifier: expected_target_system_id: 00000001 actual_target_system_id=FFFFFFFF")
	})
	t.Run("Invalid target recordID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id: transferFC.TargetRecordId=6E6F7420657175616C20746F207472616E73616374696F6E2E756E69744964 tx.UnitId=0000000000000000000000000000000000000000000000000000000000000001FF")
	})
	t.Run("Invalid target unit backlink (fee credit record does not exist)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("non-empty target unit backlink")))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink=6E6F6E2D656D7074792074617267657420756E6974206261636B6C696E6B unit.backlink=")
	})
	t.Run("Invalid target unit backlink (tx target unit backlink equals to fee credit record state hash and NOT backlink)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("sent target unit backlink")))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Backlink: []byte("actual target unit backlink")}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit backlink: transferFC.targetUnitBacklink=73656E742074617267657420756E6974206261636B6C696E6B unit.backlink=61637475616C2074617267657420756E6974206261636B6C696E6B")
	})
	t.Run("ok target unit backlink (tx target unit backlink equals fee credit record)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("actual target unit backlink")))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Backlink: []byte("actual target unit backlink")}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("EarliestAdditionTime in the future NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(11))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx), "invalid transferFC timeout: earliest=11 latest=10 current=10")
	})
	t.Run("EarliestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(9))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC timeout: earliest=0 latest=9 current=10")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 90}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 10}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCTx(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNr: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"proof is not valid: proof block hash does not match to block hash in unicity certificate")
	})
}

func TestAddFC_ExecuteAddNewFeeCredit(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	feeCreditModule := newTestFeeModule(t, trustBase)

	attr := testfc.NewAddFCAttr(t, signer)
	tx := testfc.NewAddFC(t, signer, attr)

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 49, fcr.Balance)                      // transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // new unit is created in unlocked status

}

func TestAddFC_ExecuteUpdateExistingFeeCreditRecord(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	attr := testfc.NewAddFCAttr(t, signer)
	tx := testfc.NewAddFC(t, signer, attr)
	existingFCR := &fc.FeeCreditRecord{Balance: 10, Backlink: nil, Locked: 1}
	feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, existingFCR))

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10}))
	sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 59, fcr.Balance)                      // existing (10) + transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, tx.Hash(crypto.SHA256), fcr.Backlink) // backlink is set to txhash
	require.EqualValues(t, 11, fcr.Timeout)                      // transferFC.latestAdditionTime + 1
	require.EqualValues(t, 0, fcr.Locked)                        // unit is unlocked
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	addFC := testfc.NewAddFC(t, signer, nil)
	attr := &fc.AddFeeCreditAttributes{}
	require.NoError(t, addFC.UnmarshalAttributes(attr))

	attr.FeeCreditTransferProof.BlockHeaderHash = []byte("invalid hash")
	return attr.FeeCreditTransferProof
}
