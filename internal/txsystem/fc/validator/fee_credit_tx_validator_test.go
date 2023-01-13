package validator

import (
	"crypto"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
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
		tx          *fc.AddFeeCreditWrapper
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
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{FeeCreditRecordId: recordID}),
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
					testfc.WithTransferFCTx(testfc.NewTransferFC(t, nil,
						testtransaction.WithSystemID([]byte("not money partition")),
					).Transaction),
				),
			),
			wantErr: ErrAddFCInvalidSystemID,
		},
		{
			name: "Invalid target systemID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithTargetSystemID([]byte("not money partition"))),
					).Transaction),
				),
			),
			wantErr: ErrAddFCInvalidTargetSystemID,
		},
		{
			name: "Invalid target recordID",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithTargetRecordID([]byte("not equal to transaction.unitId"))),
					).Transaction),
				),
			),
			wantErr: ErrAddFCInvalidTargetRecordID,
		},
		{
			name: "Invalid nonce",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithNonce([]byte("non-empty nonce"))),
					).Transaction),
				),
			),
			wantErr: ErrAddFCInvalidNonce,
		},
		{
			name: "EarliestAdditionTime in the future NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(10)),
					).Transaction),
				),
			),
			roundNumber: 10,
			wantErr:     ErrAddFCInvalidTimeout,
		},
		{
			name: "EarliestAdditionTime next block ok",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithEarliestAdditionTime(10)),
					).Transaction),
				),
			),
			roundNumber: 9,
			wantErr:     nil,
		},
		{
			name: "LatestAdditionTime in the past NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(10)),
					).Transaction),
				),
			),
			roundNumber: 10,
			wantErr:     ErrAddFCInvalidTimeout,
		},
		{
			name: "LatestAdditionTime next block ok",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(10)),
					).Transaction),
				),
			),
			roundNumber: 9,
			wantErr:     nil,
		},
		{
			name: "LatestAdditionTime in the past NOK",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithLatestAdditionTime(10)),
					).Transaction),
				),
			),
			roundNumber: 10,
			wantErr:     ErrAddFCInvalidTimeout,
		},
		{
			name: "Invalid tx fee",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCTx(testfc.NewTransferFC(t,
						testfc.NewTransferFCAttr(testfc.WithAmount(100)),
					).Transaction),
				),
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{MaxFee: 101}),
			),
			roundNumber: 5,
			wantErr:     ErrAddFCInvalidTxFee,
		},
		{
			name: "Invalid block proof",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer,
					testfc.WithTransferFCProof(newInvalidProof(t, signer)),
				),
			),
			roundNumber: 5,
			wantErrMsg:  "proof verification failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAddFC(&AddFCValidationContext{
				Tx:                 tt.tx,
				Unit:               tt.unit,
				currentRoundNumber: tt.roundNumber,
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
		tx         *fc.CloseFeeCreditWrapper
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			unit:    &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: 50}},
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: nil,
		},
		{
			name: "RecordID exists",
			unit: &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: 50}},
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{FeeCreditRecordId: recordID}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			unit: &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: 50}},
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
			name:    "Invalid balance",
			unit:    &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: -1}},
			tx:      testfc.NewCloseFC(t, nil),
			wantErr: ErrCloseFCInvalidBalance,
		},
		{
			name:    "Invalid amount",
			unit:    &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: 50}},
			tx:      testfc.NewCloseFC(t, testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(51))),
			wantErr: ErrCloseFCInvalidAmount,
		},
		{
			name: "Invalid fee",
			unit: &rma.Unit{Data: &txsystem.FeeCreditRecord{Balance: 50}},
			tx: testfc.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{MaxFee: 51}),
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

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *block.BlockProof {
	addFC := testfc.NewAddFC(t, signer, nil)
	addFC.AddFC.FeeCreditTransferProof.TransactionsHash = []byte("invalid hash")
	return addFC.AddFC.FeeCreditTransferProof
}

type testData struct {
}

func (t *testData) AddToHasher(_ hash.Hash) {
}

func (t *testData) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}
