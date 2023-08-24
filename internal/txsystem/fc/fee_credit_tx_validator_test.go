package fc

import (
	"crypto"
	"hash"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var (
	moneySystemID = []byte{0, 0, 0, 0}
	systemID      = []byte{0, 0, 0, 0}
	recordID      = []byte{0}
	feeProof      = []byte{1}
	bearer        = []byte{2}
)

func TestAddFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers)

	tests := []struct {
		name        string
		unit        *state.Unit
		tx          *types.TransactionOrder
		roundNumber uint64
		wantErrMsg  string
	}{
		{
			name:        "Ok",
			tx:          testfc.NewAddFC(t, signer, nil),
			roundNumber: 5,
		},
		{
			name: "transferFC tx record is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&transactions.AddFeeCreditAttributes{FeeCreditTransfer: nil})),
			wantErrMsg: "transferFC tx record is nil",
		},
		{
			name: "transferFC tx order is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&transactions.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}},
			)),
			wantErrMsg: "transferFC tx order is nil",
		},
		{
			name: "transferFC proof is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&transactions.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{}}, FeeCreditTransferProof: nil},
			)),
			wantErrMsg: "transferFC tx proof is nil",
		},
		{
			name: "RecordID exists",
			unit: nil,
			tx: testfc.NewAddFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErrMsg: "fee tx cannot contain fee credit reference",
		},
		{
			name: "Fee proof exists",
			unit: nil,
			tx: testfc.NewAddFC(t, signer, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErrMsg: "fee tx cannot contain fee authorization proof",
		},
		{
			name:       "Invalid unit type",
			unit:       state.NewUnit(bearer, &testData{}), // add unit with wrong type
			tx:         testfc.NewAddFC(t, signer, nil),
			wantErrMsg: "invalid unit type: unit is not fee credit record",
		},
		{
			name: "Invalid fee credit owner condition",
			unit: state.NewUnit(bearer, &unit.FeeCreditRecord{}),
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithFCOwnerCondition([]byte("wrong bearer")),
				),
			),
			wantErrMsg: "invalid owner condition",
		},
		{
			name: "Invalid systemID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, nil, testtransaction.WithSystemID([]byte("not money partition"))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC system identifier",
		},
		{
			name: "Invalid target systemID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID([]byte("not money partition")))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target system identifier",
		},
		{
			name: "Invalid target recordID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target record id",
		},
		{
			name: "Invalid target unit backlink (fee credit record does not exist)",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("non-empty target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target unit backlink",
		},
		{
			name: "Invalid target unit backlink (tx target unit backlink equals to fee credit record state hash and NOT backlink)",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("sent target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			unit:       state.NewUnit(nil, &unit.FeeCreditRecord{Hash: []byte("actual target unit backlink")}),
			wantErrMsg: "invalid transferFC target unit backlink",
		},
		{
			name: "ok target unit backlink (tx target unit backlink equals fee credit record)",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetUnitBacklink([]byte("actual target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			unit: state.NewUnit(nil, &unit.FeeCreditRecord{Hash: []byte("actual target unit backlink")}),
		},
		{
			name: "EarliestAdditionTime in the future NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(11))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			roundNumber: 10,
			wantErrMsg:  "invalid transferFC timeout",
		},
		{
			name: "EarliestAdditionTime next block OK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(10))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			roundNumber: 10,
		},
		{
			name: "LatestAdditionTime in the past NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(9))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			roundNumber: 10,
			wantErrMsg:  "invalid transferFC timeout",
		},
		{
			name: "LatestAdditionTime next block OK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(10))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			roundNumber: 10,
		},
		{
			name: "Invalid tx fee",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
				testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
			),
			roundNumber: 5,
			wantErrMsg:  "invalid transferFC fee",
		},
		{
			name: "Invalid tx proof",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCProof(newInvalidProof(t, signer)),
				),
			),
			roundNumber: 5,
			wantErrMsg:  "proof is not valid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAddFeeCredit(&AddFCValidationContext{
				Tx:                 tt.tx,
				Unit:               tt.unit,
				CurrentRoundNumber: tt.roundNumber,
			})
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCloseFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers)

	tests := []struct {
		name       string
		unit       *state.Unit
		tx         *types.TransactionOrder
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			unit:    state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: nil,
		},
		{
			name:       "tx is nil",
			unit:       state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx:         nil,
			wantErrMsg: "tx is nil",
		},
		{
			name: "RecordID exists",
			unit: state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErrMsg: "fee tx cannot contain fee credit reference",
		},
		{
			name: "Fee proof exists",
			unit: state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErrMsg: "fee tx cannot contain fee authorization proof",
		},
		{
			name:       "Invalid unit nil",
			unit:       nil,
			tx:         testfc.NewCloseFC(t, nil),
			wantErrMsg: "unit is nil",
		},
		{
			name:       "Invalid unit type",
			unit:       state.NewUnit(nil, &testData{}),
			tx:         testfc.NewCloseFC(t, nil),
			wantErrMsg: "unit data is not of type fee credit record",
		},
		{
			name:       "Invalid amount",
			unit:       state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx:         testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51))),
			wantErrMsg: "invalid amount",
		},
		{
			name: "Invalid fee",
			unit: state.NewUnit(nil, &unit.FeeCreditRecord{Balance: 50}),
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}),
			),
			wantErrMsg: "invalid fee",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCloseFC(&CloseFCValidationContext{
				Tx:   tt.tx,
				Unit: tt.unit,
			})
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	addFC := testfc.NewAddFC(t, signer, nil)
	attr := &transactions.AddFeeCreditAttributes{}
	require.NoError(t, addFC.UnmarshalAttributes(attr))

	attr.FeeCreditTransferProof.BlockHeaderHash = []byte("invalid hash")
	return attr.FeeCreditTransferProof
}

type testData struct {
}

func (t *testData) Write(hash.Hash) {
}

func (t *testData) SummaryValueInput() uint64 {
	return 0
}

func (t *testData) Copy() state.UnitData {
	return &testData{}
}
