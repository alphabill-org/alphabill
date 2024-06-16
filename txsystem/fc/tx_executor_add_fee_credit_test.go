package fc

import (
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/predicates"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestFeeCredit_validateCreateFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(testfc.NewFeeCreditRecordID(t, signer)),
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{FeeCreditTransfer: nil, FeeCreditTransferProof: &types.TxProof{}}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: transferFC tx record is nil")
	})
	t.Run("transferFC tx order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
			&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}, FeeCreditTransferProof: &types.TxProof{}}))
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: transferFC tx order is nil")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transferFC tx proof is nil")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transferFC tx order is missing server metadata")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: invalid transfer fee credit transaction payload type: addFC")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: failed to unmarshal transfer payload: cbor: cannot unmarshal array into Go value of type fc.TransferFeeCreditAttributes (cannot decode CBOR array to struct with different number of elements)")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"fee credit tx validation error: fee tx cannot contain fee credit reference")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id")
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer))
		feeCreditModule := newTestFeeModule(t, trustBase, withFeePredicateRunner(func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) error {
			return fmt.Errorf("predicate error")
		}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"executing fee credit predicate: predicate error")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC money system identifier FFFFFFFF (expected 00000001)")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC target system identifier: expected_target_system_id: 00000001 actual_target_system_id=FFFFFFFF")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target unit counter (target counter must be nil if creating fee credit record for the first time)")
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithLatestAdditionTime(10))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC timeout: latestAdditionTime=10 currentRoundNumber=11")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase)
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"proof is not valid: proof block hash does not match to block hash in unicity certificate")
	})

}

func TestAddFC_ValidateAddNewFeeCreditTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	pubkey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	t.Run("ok - empty", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
	})
	t.Run("err - replay will not pass validation", func(t *testing.T) {
		feeCreditModule := newTestFeeModule(t, trustBase)
		attr := testfc.NewAddFCAttr(t, signer)
		tx := testfc.NewAddFC(t, signer, attr)
		require.NoError(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
		sm, err := feeCreditModule.executeAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
		require.NoError(t, err)
		require.NotNil(t, sm)
		// replay attack - have to use error contains since backlink hash changes
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))),
			"invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
	})
	t.Run("transferFC tx record is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(testfc.NewFeeCreditRecordID(t, signer)),
			testtransaction.WithAttributes(&fc.AddFeeCreditAttributes{FeeCreditTransfer: nil, FeeCreditTransferProof: &types.TxProof{}}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: transferFC tx record is nil")
	})
	t.Run("transferFC tx order is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(testfc.NewFeeCreditRecordID(t, signer)),
			testtransaction.WithAttributes(
				&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}, FeeCreditTransferProof: &types.TxProof{}}))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: transferFC tx order is nil")
	})
	t.Run("transferFC proof is nil", func(t *testing.T) {
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(testfc.NewFeeCreditRecordID(t, signer)),
			testtransaction.WithAttributes(
				&fc.AddFeeCreditAttributes{
					FeeCreditTransfer: &types.TransactionRecord{
						TransactionOrder: &types.TransactionOrder{},
					},
					FeeCreditTransferProof: nil,
				},
			))
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transferFC tx proof is nil")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transferFC tx order is missing server metadata")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: invalid transfer fee credit transaction payload type: addFC")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: transfer tx attributes error: failed to unmarshal transfer payload: cbor: cannot unmarshal array into Go value of type fc.TransferFeeCreditAttributes (cannot decode CBOR array to struct with different number of elements)")
	})
	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil,
			testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
		)
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"fee credit tx validation error: fee tx cannot contain fee credit reference")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC target record id:")
	})
	t.Run("Invalid unit type", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer, nil, testtransaction.WithUnitID([]byte{1}))
		feeCreditModule := newTestFeeModule(t, trustBase,
			withFeeCreditType([]byte{0xff}),
			withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"get fcr error: invalid unit identifier: type is not fee credit record")
	})
	t.Run("Invalid fee credit owner condition", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithTargetUnitCounter(10))),
						ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
					},
				),
			))
		feePredicateRunner := func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) error {
			return fmt.Errorf("predicate error")
		}
		feeCreditModule := newTestFeeModule(t, trustBase,
			withFeePredicateRunner(feePredicateRunner),
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey([]byte{1, 2, 3}), &fc.FeeCreditRecord{Balance: 10, Counter: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid owner condition:")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC money system identifier FFFFFFFF (expected 00000001)")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC target system identifier: expected_target_system_id: 00000001 actual_target_system_id=FFFFFFFF")
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
		feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.ErrorContains(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"invalid transferFC target record id")
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
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
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
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
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey(pubkey), &fc.FeeCreditRecord{Counter: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.NoError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx))
	})
	t.Run("LatestAdditionTime in the past NOK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer, testfc.WithLatestAdditionTime(10))),
						ServerMetadata:   &types.ServerMetadata{},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey(pubkey), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC timeout: latestAdditionTime=10 currentRoundNumber=11")
	})
	t.Run("LatestAdditionTime next block OK", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetUnitCounter(10),
							testfc.WithLatestAdditionTime(10))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
			),
		)
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey(pubkey), &fc.FeeCreditRecord{Balance: 10, Counter: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))
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
		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey(pubkey), &fc.FeeCreditRecord{Balance: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
		var attr fc.AddFeeCreditAttributes
		require.NoError(t, tx.UnmarshalAttributes(&attr))
		require.EqualError(t, feeCreditModule.validateAddFC(tx, &attr, execCtx),
			"add fee credit validation failed: invalid transferFC fee: max_fee+actual_fee=102 transferFC.Amount=100")
	})
	t.Run("Invalid proof", func(t *testing.T) {
		tx := testfc.NewAddFC(t, signer,
			testfc.NewAddFCAttr(t, signer,
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
				testfc.WithTransferFCRecord(
					&types.TransactionRecord{
						TransactionOrder: testfc.NewTransferFC(t, signer, testfc.NewTransferFCAttr(t, signer,
							testfc.WithTargetUnitCounter(10),
							testfc.WithLatestAdditionTime(10))),
						ServerMetadata: &types.ServerMetadata{ActualFee: 1},
					},
				),
				testfc.WithTransferFCProof(newInvalidProof(t, signer)),
			),
		)

		feeCreditModule := newTestFeeModule(t, trustBase,
			withStateUnit(tx.UnitID(), templates.NewP2pkh256BytesFromKey(pubkey), &fc.FeeCreditRecord{Balance: 10, Counter: 10}))
		execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(5))
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

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
	sm, err := feeCreditModule.executeAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
	feeCreditModule := newTestFeeModule(t, trustBase, withStateUnit(tx.UnitID(), attr.FeeCreditOwnerCondition, existingFCR))

	require.NoError(t, feeCreditModule.validateAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10))))
	sm, err := feeCreditModule.executeAddFC(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
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
