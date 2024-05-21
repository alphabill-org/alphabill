package fc

import (
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/txsystem"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestAddFC_ValidateAddNewFeeCreditTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}))
		sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10})
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}),
			"invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(testfc.NewFeeCreditRecordID(t, signer)),
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{FeeCreditTransfer: nil}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx record is nil")
	})
	t.Run("transferFC tx order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}}))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx proof is nil")
	})
	t.Run("transferFC server metadata is nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(10))),
						ServerMetadata:   nil,
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"transferFC tx order is missing server metadata")
	})
	t.Run("transferFC type not valid", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewAddFC(t, signer, nil),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transfer fee credit transaction payload type: addFC")
	})
	t.Run("transferFC attributes unmarshal error", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewAddFC(t, signer, nil,
							testtransaction.WithPayloadType(fc.PayloadTypeTransferFeeCredit)),
						ServerMetadata: &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid fee credit transaction: fee tx cannot contain fee credit reference")
	})
	t.Run("bill not transferred to fee credits of the target record", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(&types.TransactionRecord{
					TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetRecordID([]byte{1}))),
					ServerMetadata:   &types.ServerMetadata{},
				}),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil, testtransaction.WithUnitID([]byte{1}))
		feeCreditModule := newTestFeeModule(t, trustBase, withFeeCreditType([]byte{0xff}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"get fcr error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithFCOwnerCondition([]byte("wrong bearer")),
			))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid owner condition: expected=83004101F6 actual=77726F6E6720626561726572")
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
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"addFC: invalid transferFC money system identifier FFFFFFFF (expected 00000001)")
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
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target system identifier: expected_target_system_id: 00000001 actual_target_system_id=FFFFFFFF")
	})
	t.Run("Invalid target recordID", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id")
	})
	t.Run("invalid target unit counter (fee credit record does not exist, counter must be nil)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit counter (target counter must be nil if creating fee credit record for the first time)")
	})
	t.Run("invalid target unit counter (fee credit record exist, counter must be non-nil)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, nil), // default counter is nil
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Counter: 11}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
	})
	t.Run("invalid target unit counter (both exist but are not equal)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Counter: 11}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			fmt.Sprintf("invalid transferFC target unit counter: transferFC.targetUnitCounter=%d unit.counter=%d", 10, 11))

	})
	t.Run("ok target unit counter (tx target unit counter equals fee credit record counter)", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), nil, &fc.FeeCreditRecord{Counter: 10}))
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer)),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 11}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC timeout: latestAdditionTime=10 currentRoundNumber=11")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithLatestAdditionTime(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 10}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("Invalid tx fee", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithAmount(100))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
			testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
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
		execCtx := &txsystem.TxExecutionContext{CurrentBlockNumber: 5}
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"proof is not valid: proof block hash does not match to block hash in unicity certificate")
	})
}

func TestAddFC_ExecuteAddNewFeeCredit(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	feeCreditModule := newTestFeeModule(t, trustBase)
	attr := testfc.NewAddFCAttr(t, signer)
	tx := testfc.NewAddFC(t, signer, attr)

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}))
	sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 49, fcr.Balance) // transferFC.amount (50) - transferFC.fee (0) - addFC.fee (1)
	require.EqualValues(t, 0, fcr.Counter)  // new unit counter starts from 0
	require.EqualValues(t, 10, fcr.Timeout) // transferFC.latestAdditionTime
	require.EqualValues(t, 0, fcr.Locked)   // new unit is created in unlocked status

}

func TestAddFC_ExecuteUpdateExistingFeeCreditRecord(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	attr := testfc.NewAddFCAttr(t, signer,
		testfc.WithTransferFCRecord(
			&types.TransactionRecord{
				TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
					testfc.WithAmount(50),
					testfc.WithTargetUnitCounter(4),
					testfc.WithTargetRecordID(testfc.NewFeeCreditRecordID(t, signer)),
				)),
				ServerMetadata: &types.ServerMetadata{ActualFee: 1},
			},
		),
	)
	tx := testfc.NewAddFC(t, signer, attr)
	existingFCR := &fc.FeeCreditRecord{Balance: 10, Counter: 4, Locked: 1}
	feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), nil, existingFCR))

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10}))
	sm, err := feeCreditModule.executeAddFC(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNumber: 10})
	require.NoError(t, err)
	require.NotNil(t, sm)

	u, err := feeCreditModule.state.GetUnit(tx.UnitID(), false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 58, fcr.Balance) // existing (10) + transferFC.amount (50) - transferFC.fee (1) - addFC.fee (1)
	require.EqualValues(t, 5, fcr.Counter)  // counter is incremented
	require.EqualValues(t, 10, fcr.Timeout) // transferFC.latestAdditionTime
	require.EqualValues(t, 0, fcr.Locked)   // unit is unlocked
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	addFC := testfc.NewAddFC(t, signer, nil)
	attr := &fc.AddFeeCreditAttributes{}
	require.NoError(t, addFC.UnmarshalAttributes(attr))

	attr.FeeCreditTransferProof.BlockHeaderHash = []byte("invalid hash")
	return attr.FeeCreditTransferProof
}
