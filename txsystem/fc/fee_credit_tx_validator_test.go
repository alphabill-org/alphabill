package fc

import (
	"crypto"
	"hash"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

var (
	moneySystemID           types.SystemID = 0x00000001
	systemID                types.SystemID = 0x00000001
	recordID                               = []byte{0}
	feeProof                               = []byte{1}
	bearer                                 = []byte{2}
	feeCreditRecordUnitType                = []byte{0xff}
)

func TestAddFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers, feeCreditRecordUnitType)

	tests := []struct {
		name        string
		unit        *state.Unit
		tx          *types.TransactionOrder
		roundNumber uint64
		wantErrMsg  string
	}{
		{
			name:        "Ok",
			tx:          testutils.NewAddFC(t, signer, nil),
			roundNumber: 5,
		},
		{
			name: "transferFC tx record is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&fc.AddFeeCreditAttributes{FeeCreditTransfer: nil})),
			wantErrMsg: "transferFC tx record is nil",
		},
		{
			name: "transferFC tx order is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: nil}},
			)),
			wantErrMsg: "transferFC tx order is nil",
		},
		{
			name: "transferFC proof is nil",
			tx: testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(
				&fc.AddFeeCreditAttributes{FeeCreditTransfer: &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{}}, FeeCreditTransferProof: nil},
			)),
			wantErrMsg: "transferFC tx proof is nil",
		},
		{
			name: "FeeCreditRecordID is not nil",
			unit: nil,
			tx: testutils.NewAddFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErrMsg: "fee tx cannot contain fee credit reference",
		},
		{
			name: "UnitID has wrong type",
			unit: nil,
			tx: testutils.NewAddFC(t, signer, nil,
				testtransaction.WithUnitID([]byte{1}),
			),
			wantErrMsg: "invalid unit identifier",
		},
		{
			name: "Fee proof exists",
			unit: nil,
			tx: testutils.NewAddFC(t, signer, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErrMsg: "fee tx cannot contain fee authorization proof",
		},
		{
			name:       "Invalid unit type",
			unit:       state.NewUnit(bearer, &testData{}), // add unit with wrong type
			tx:         testutils.NewAddFC(t, signer, nil),
			wantErrMsg: "invalid unit type: unit is not fee credit record",
		},
		{
			name: "Invalid fee credit owner condition",
			unit: state.NewUnit(bearer, &fc.FeeCreditRecord{}),
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithFCOwnerCondition([]byte("wrong bearer")),
				),
			),
			wantErrMsg: "invalid owner condition",
		},
		{
			name: "Invalid systemID",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, nil, testtransaction.WithSystemID(0xFFFFFFFF)),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: `addFC: invalid transferFC money system identifier FFFFFFFF (expected 00000001)`,
		},
		{
			name: "Invalid target systemID",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetSystemID(0xFFFFFFFF))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target system identifier",
		},
		{
			name: "Invalid target recordID",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID([]byte("not equal to transaction.unitId")))),
							ServerMetadata:   &types.ServerMetadata{},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target record id",
		},
		{
			name: "Invalid target unit backlink (fee credit record does not exist)",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetUnitBacklink([]byte("non-empty target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			wantErrMsg: "invalid transferFC target unit backlink",
		},
		{
			name: "Invalid target unit backlink (tx target unit backlink equals to fee credit record state hash and NOT backlink)",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetUnitBacklink([]byte("sent target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			unit:       state.NewUnit(nil, &fc.FeeCreditRecord{Backlink: []byte("actual target unit backlink")}),
			wantErrMsg: "invalid transferFC target unit backlink",
		},
		{
			name: "ok target unit backlink (tx target unit backlink equals fee credit record)",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetUnitBacklink([]byte("actual target unit backlink")))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			unit: state.NewUnit(nil, &fc.FeeCreditRecord{Backlink: []byte("actual target unit backlink")}),
		},
		{
			name: "EarliestAdditionTime in the future NOK",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithEarliestAdditionTime(11))),
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
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithEarliestAdditionTime(10))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			roundNumber: 10,
		},
		{
			name: "LatestAdditionTime in the past NOK",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithLatestAdditionTime(9))),
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
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithLatestAdditionTime(10))),
							ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
						},
					),
				),
			),
			roundNumber: 10,
		},
		{
			name: "Invalid tx fee",
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(100))),
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
			tx: testutils.NewAddFC(t, signer,
				testutils.NewAddFCAttr(t, signer,
					testutils.WithTransferFCProof(newInvalidProof(t, signer)),
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
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers, feeCreditRecordUnitType)

	tests := []struct {
		name       string
		unit       *state.Unit
		tx         *types.TransactionOrder
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			unit:    state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx:      testutils.NewCloseFC(t, nil),
			wantErr: nil,
		},
		{
			name:       "tx is nil",
			unit:       state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx:         nil,
			wantErrMsg: "tx is nil",
		},
		{
			name: "FeeCreditRecordID is not nil",
			unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx: testutils.NewCloseFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
			),
			wantErrMsg: "fee tx cannot contain fee credit reference",
		},
		{
			name: "UnitID has wrong type",
			unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx: testutils.NewCloseFC(t, nil,
				testtransaction.WithUnitID([]byte{8}),
			),
			wantErrMsg: "invalid unit identifier",
		},
		{
			name: "Fee proof exists",
			unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx: testutils.NewCloseFC(t, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErrMsg: "fee tx cannot contain fee authorization proof",
		},
		{
			name:       "Invalid unit nil",
			unit:       nil,
			tx:         testutils.NewCloseFC(t, nil),
			wantErrMsg: "unit is nil",
		},
		{
			name:       "Invalid unit type",
			unit:       state.NewUnit(nil, &testData{}),
			tx:         testutils.NewCloseFC(t, nil),
			wantErrMsg: "unit data is not of type fee credit record",
		},
		{
			name:       "Invalid amount",
			unit:       state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx:         testutils.NewCloseFC(t, testutils.NewCloseFCAttr(testutils.WithCloseFCAmount(51))),
			wantErrMsg: "invalid amount",
		},
		{
			name:       "Nil target unit id",
			unit:       state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx:         testutils.NewCloseFC(t, testutils.NewCloseFCAttr(testutils.WithCloseFCTargetUnitID(nil))),
			wantErrMsg: "TargetUnitID is empty",
		},
		{
			name:       "Empty target unit id",
			unit:       state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx:         testutils.NewCloseFC(t, testutils.NewCloseFCAttr(testutils.WithCloseFCTargetUnitID([]byte{}))),
			wantErrMsg: "TargetUnitID is empty",
		},
		{
			name: "Invalid fee",
			unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50}),
			tx: testutils.NewCloseFC(t, nil,
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

func TestLockFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers, feeCreditRecordUnitType)

	tests := []struct {
		name string
		ctx  *LockFCValidationContext
		err  string
	}{
		{
			name: "Ok",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, nil),
			),
		},
		{
			name: "unit is nil",
			ctx: newLockFCValidationContext(
				nil,
				testutils.NewLockFC(t, nil),
			),
			err: "unit is nil",
		},
		{
			name: "tx is nil",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				nil,
			),
			err: "tx is nil",
		},
		{
			name: "attr is nil",
			ctx: &LockFCValidationContext{
				Tx:   testutils.NewLockFC(t, nil),
				Attr: nil,
				Unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
			},
			err: "tx attributes is nil",
		},
		{
			name: "unit id type part is not fee credit record",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, testutils.NewLockFCAttr(), testtransaction.WithUnitID(
					types.NewUnitID(33, nil, []byte{1}, []byte{0xfe})),
				),
			),
			err: "invalid unit identifier: type is not fee credit record",
		},
		{
			name: "invalid unit data type",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &testData{}),
				testutils.NewLockFC(t, nil),
			),
			err: "invalid unit type: unit is not fee credit record",
		},
		{
			name: "FCR is already locked",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewLockFC(t, nil),
			),
			err: "fee credit record is already locked",
		},
		{
			name: "lock status is zero",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 0}),
				testutils.NewLockFC(t, testutils.NewLockFCAttr(testutils.WithLockStatus(0))),
			),
			err: "lock status must be non-zero value",
		},
		{
			name: "invalid backlink",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, testutils.NewLockFCAttr(testutils.WithLockFCBacklink([]byte{3}))),
			),
			err: "the transaction backlink does not match with fee credit record backlink",
		},
		{
			name: "max fee exceeds balance",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, nil,
					testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}),
				),
			),
			err: "max fee cannot exceed fee credit record balance",
		},
		{
			name: "FeeCreditRecordID is not nil",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, nil,
					testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
				),
			),
			err: "fee tx cannot contain fee credit reference",
		},
		{
			name: "fee proof is not nil",
			ctx: newLockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewLockFC(t, nil,
					testtransaction.WithFeeProof(feeProof),
				),
			),
			err: "fee tx cannot contain fee authorization proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateLockFC(tt.ctx)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUnlockFC(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}
	validator := NewDefaultFeeCreditTxValidator(moneySystemID, systemID, crypto.SHA256, verifiers, feeCreditRecordUnitType)

	tests := []struct {
		name string
		ctx  *UnlockFCValidationContext
		err  string
	}{
		{
			name: "Ok",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewUnlockFC(t, nil),
			),
		},
		{
			name: "unit is nil",
			ctx: newUnlockFCValidationContext(
				nil,
				testutils.NewUnlockFC(t, nil),
			),
			err: "unit is nil",
		},
		{
			name: "tx is nil",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				nil,
			),
			err: "tx is nil",
		},
		{
			name: "attr is nil",
			ctx: &UnlockFCValidationContext{
				Tx:   testutils.NewUnlockFC(t, nil),
				Attr: nil,
				Unit: state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
			},
			err: "tx attributes is nil",
		},
		{
			name: "unit id type part is not fee credit record",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}}),
				testutils.NewUnlockFC(t, testutils.NewUnlockFCAttr(), testtransaction.WithUnitID(
					types.NewUnitID(33, nil, []byte{1}, []byte{0xfe})),
				),
			),
			err: "invalid unit identifier: type is not fee credit record",
		},
		{
			name: "invalid unit data type",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &testData{}),
				testutils.NewUnlockFC(t, nil),
			),
			err: "invalid unit type: unit is not fee credit record",
		},
		{
			name: "FCR is already unlocked",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 0}),
				testutils.NewUnlockFC(t, nil),
			),
			err: "fee credit record is already unlock",
		},
		{
			name: "invalid backlink",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewUnlockFC(t, testutils.NewUnlockFCAttr(testutils.WithUnlockFCBacklink([]byte{3}))),
			),
			err: "the transaction backlink does not match with fee credit record backlink",
		},
		{
			name: "max fee exceeds balance",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewUnlockFC(t, nil,
					testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 51}),
				),
			),
			err: "max fee cannot exceed fee credit record balance",
		},
		{
			name: "FeeCreditRecordID is not nil",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewUnlockFC(t, nil,
					testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
				),
			),
			err: "fee tx cannot contain fee credit reference",
		},
		{
			name: "fee proof is not nil",
			ctx: newUnlockFCValidationContext(
				state.NewUnit(nil, &fc.FeeCreditRecord{Balance: 50, Backlink: []byte{4}, Locked: 1}),
				testutils.NewUnlockFC(t, nil,
					testtransaction.WithFeeProof(feeProof),
				),
			),
			err: "fee tx cannot contain fee authorization proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateUnlockFC(tt.ctx)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func newLockFCValidationContext(unit *state.Unit, tx *types.TransactionOrder) *LockFCValidationContext {
	var attr *fc.LockFeeCreditAttributes
	if tx != nil {
		_ = tx.Payload.UnmarshalAttributes(&attr)
	}
	return &LockFCValidationContext{
		Tx:   tx,
		Attr: attr,
		Unit: unit,
	}
}

func newUnlockFCValidationContext(unit *state.Unit, tx *types.TransactionOrder) *UnlockFCValidationContext {
	var attr *fc.UnlockFeeCreditAttributes
	if tx != nil {
		_ = tx.Payload.UnmarshalAttributes(&attr)
	}
	return &UnlockFCValidationContext{
		Tx:   tx,
		Attr: attr,
		Unit: unit,
	}
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	addFC := testutils.NewAddFC(t, signer, nil)
	attr := &fc.AddFeeCreditAttributes{}
	require.NoError(t, addFC.UnmarshalAttributes(attr))

	attr.FeeCreditTransferProof.BlockHeaderHash = []byte("invalid hash")
	return attr.FeeCreditTransferProof
}

type testData struct {
}

func (t *testData) Write(hash.Hash) error {
	return nil
}

func (t *testData) SummaryValueInput() uint64 {
	return 0
}

func (t *testData) Copy() types.UnitData {
	return &testData{}
}
