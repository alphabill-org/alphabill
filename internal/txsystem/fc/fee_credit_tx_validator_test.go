package fc

import (
	"crypto"
	"hash"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
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
		unit        *rma.Unit
		tx          *types.TransactionOrder
		roundNumber uint64
		wantErr     error
		wantErrMsg  string
	}{
		{
			name:        "Ok",
			tx:          testfc.NewAddFC(t, signer, nil),
			roundNumber: 5,
			wantErr:     nil,
		},
		{
			name: "RecordID exists",
			unit: nil,
			tx: testfc.NewAddFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			unit: nil,
			tx: testfc.NewAddFC(t, signer, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name: "Invalid fee credit owner condition",
			unit: &rma.Unit{Bearer: bearer},
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithFCOwnerCondition([]byte("wrong bearer")),
				),
			),
			wantErr: ErrAddFCInvalidOwnerCondition,
		},
		{
			name: "Invalid systemID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, nil, testtransaction.WithSystemID([]byte("not money partition"))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			wantErr: ErrAddFCInvalidSystemID,
		},
		{
			name: "Invalid target systemID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID([]byte("not money partition")))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			wantErr: ErrAddFCInvalidTargetSystemID,
		},
		{
			name: "Invalid target recordID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			wantErr: ErrAddFCInvalidTargetRecordID,
		},
		{
			name: "Invalid nonce",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithNonce([]byte("non-empty nonce")))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			wantErr: ErrAddFCInvalidNonce,
		},
		{
			name: "EarliestAdditionTime in the future NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(11))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			roundNumber: 10,
			wantErr:     ErrAddFCInvalidTimeout,
		},
		{
			name: "EarliestAdditionTime next block OK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(10))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			roundNumber: 10,
			wantErr:     nil,
		},
		{
			name: "LatestAdditionTime in the past NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(9))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			roundNumber: 10,
			wantErr:     ErrAddFCInvalidTimeout,
		},
		{
			name: "LatestAdditionTime next block OK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(10))),
							ServerMetadata:   nil,
						},
					),
				),
			),
			roundNumber: 10,
			wantErr:     nil,
		},
		{
			name: "Invalid tx fee",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(100))),
							ServerMetadata:   nil,
						},
					),
				),
				testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 101}),
			),
			roundNumber: 5,
			wantErr:     ErrAddFCInvalidTxFee,
		},
		{
			name: "Invalid tx proof",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCProof(newInvalidProof(t, signer)),
				),
			),
			roundNumber: 5,
			wantErrMsg:  "invalid proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAddFeeCredit(&AddFCValidationContext{
				Tx:                 tt.tx,
				Unit:               tt.unit,
				CurrentRoundNumber: tt.roundNumber,
			})
			if tt.wantErr == nil && tt.wantErrMsg == "" {
				require.NoError(t, err)
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
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
		unit       *rma.Unit
		tx         *types.TransactionOrder
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			unit:    &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: nil,
		},
		{
			name:    "tx is nil",
			unit:    &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx:      nil,
			wantErr: ErrTxIsNil,
		},
		{
			name: "RecordID exists",
			unit: &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			unit: &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name:    "Invalid unit nil",
			unit:    nil,
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: ErrCloseFCUnitIsNil,
		},
		{
			name:    "Invalid unit type",
			unit:    &rma.Unit{Data: &testData{}},
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: ErrCloseFCInvalidUnitType,
		},
		{
			name:    "Invalid amount",
			unit:    &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx:      testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51))),
			wantErr: ErrCloseFCInvalidAmount,
		},
		{
			name: "Invalid fee",
			unit: &rma.Unit{Data: &FeeCreditRecord{Balance: 50}},
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}),
			),
			wantErr: ErrCloseFCInvalidFee,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCloseFC(&CloseFCValidationContext{
				Tx:   tt.tx,
				Unit: tt.unit,
			})
			if tt.wantErr == nil && tt.wantErrMsg == "" {
				require.NoError(t, err)
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
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

func (t *testData) AddToHasher(_ hash.Hash) {
}

func (t *testData) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}
