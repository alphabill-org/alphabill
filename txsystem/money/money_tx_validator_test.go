package money

import (
	"crypto"
	"errors"
	"math"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.TransferAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 6},
			res:  nil,
		},
		{
			name: "LockedBill",
			bd:   &money.BillData{Locked: 1, V: 100, Counter: 6},
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 6},
			res:  ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 101, Counter: 6},
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidCounter",
			bd:   newBillData(100, 6),
			attr: &money.TransferAttributes{TargetValue: 100, Counter: 5},
			res:  ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnyTransfer(tt.bd, tt.attr.Counter, tt.attr.TargetValue)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestTransferDC(t *testing.T) {
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.TransferDCAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Counter:      6,
			},
			res: nil,
		},
		{
			name: "LockedBill",
			bd:   &money.BillData{Locked: 1, V: 100, Counter: 6},
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Counter:      6,
			},
			res: ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Counter:      6,
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidCounter",
			bd:   newBillData(100, 6),
			attr: &money.TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Counter:      7,
			},
			res: ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransferDC(tt.bd, tt.attr)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.SplitAttributes
		err  string
	}{
		{
			name: "Ok 2-way split",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Counter:        6,
			},
		},
		{
			name: "Ok 3-way split",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 80,
				Counter:        6,
			},
		},
		{
			name: "BillLocked",
			bd:   &money.BillData{Locked: 1, V: 100, Counter: 6},
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Counter:        6,
			},
			err: "bill is already locked",
		},
		{
			name: "Invalid counter",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Counter:        7,
			},
			err: ErrInvalidCounter.Error(),
		},
		{
			name: "Target units are empty",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits:    []*money.TargetUnit{},
				RemainingValue: 1,
				Counter:        6,
			},
			err: "target units are empty",
		},
		{
			name: "Target unit is nil",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits:    []*money.TargetUnit{nil},
				RemainingValue: 100,
				Counter:        6,
			},
			err: "target unit is nil",
		},
		{
			name: "Target unit amount is zero",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 0, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 100,
				Counter:        6,
			},
			err: "target unit amount is zero at index 0",
		},
		{
			name: "Target unit owner condition is empty",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 1, OwnerCondition: []byte{}},
				},
				RemainingValue: 100,
				Counter:        6,
			},
			err: "target unit owner condition is empty at index 0",
		},
		{
			name: "Target unit amount overflow",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: math.MaxUint64, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 1, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 100,
				Counter:        6,
			},
			err: "failed to add target unit amounts",
		},
		{
			name: "Remaining value is zero",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 0,
				Counter:        6,
			},
			err: "remaining value is zero",
		},
		{
			name: "Amount plus remaining value is less than bill value",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 79,
				Counter:        6,
			},
			err: "the sum of the values to be transferred plus the remaining value must equal the value of the bill",
		},
		{
			name: "Amount plus remaining value is greater than bill value",
			bd:   newBillData(100, 6),
			attr: &money.SplitAttributes{
				TargetUnits: []*money.TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 81,
				Counter:        6,
			},
			err: "the sum of the values to be transferred plus the remaining value must equal the value of the bill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplit(tt.bd, tt.attr)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

func TestSwap(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	tests := []struct {
		name string
		ctx  *swapValidationContext
		err  string
	}{
		{
			name: "Ok",
			ctx:  newSwapValidationContext(t, verifier, newSwapDC(t, signer)),
		},
		{
			name: "DC money supply < tx target value",
			ctx: newSwapValidationContext(t, verifier, newSwapDC(t, signer),
				withSwapStateUnit(string(DustCollectorMoneySupplyID), state.NewUnit(nil, &money.BillData{
					V: 99,
				}))),
			err: "insufficient DC-money supply",
		},
		{
			name: "target unit does not exist",
			ctx:  newSwapValidationContext(t, verifier, newSwapDC(t, signer), withSwapStateUnit(string([]byte{255}), nil)),
			err:  "target unit does not exist",
		},
		{
			name: "InvalidTargetValue",
			ctx:  newSwapValidationContext(t, verifier, newInvalidTargetValueSwap(t, signer)),
			err:  "target value must be equal to the sum of dust transfer values",
		},
		{
			name: "DustTransfersInDescBillIdOrder",
			ctx:  newSwapValidationContext(t, verifier, newDescBillOrderSwap(t, signer)),
			err:  "transfer orders are not listed in strictly increasing order of bill identifiers",
		},
		{
			name: "DustTransfersInEqualBillIdOrder",
			ctx:  newSwapValidationContext(t, verifier, newEqualBillIdsSwap(t, signer)),
			err:  "transfer orders are not listed in strictly increasing order of bill identifiers",
		},
		{
			name: "DustTransfersInvalidTargetSystemID",
			ctx:  newSwapValidationContext(t, verifier, newSwapOrderWithInvalidTargetSystemID(t, signer)),
			err:  "dust transfer system id is not money partition system id",
		},
		{
			name: "invalid target unit id",
			ctx:  newSwapValidationContext(t, verifier, newInvalidTargetUnitIDSwap(t, signer)),
			err:  "dust transfer order target unit id is not equal to swap tx unit id",
		},
		{
			name: "invalid target counter",
			ctx:  newSwapValidationContext(t, verifier, newInvalidTargetCounterSwap(t, signer)),
			err:  "dust transfer target counter is not equal to target unit counter",
		},
		{
			name: "InvalidProofsNil",
			ctx:  newSwapValidationContext(t, verifier, newDcProofsNilSwap(t, signer)),
			err:  "invalid count of proofs",
		},
		{
			name: "InvalidEmptyDcProof",
			ctx:  newSwapValidationContext(t, verifier, newEmptyDcProofsSwap(t, signer)),
			err:  "unicity certificate is nil",
		},
		{
			name: "InvalidDcProofInvalid",
			ctx:  newSwapValidationContext(t, verifier, newInvalidDcProofsSwap(t, signer)),
			err:  "invalid unicity seal signature",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ctx.validateSwapTx(nil)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

func TestTransferFC(t *testing.T) {
	counter := uint64(4)
	tests := []struct {
		name    string
		bd      *money.BillData
		tx      *types.TransactionOrder
		wantErr error
	}{
		{
			name:    "Ok",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, nil),
			wantErr: nil,
		},
		{
			name:    "LockedBill",
			bd:      &money.BillData{Locked: 1, V: 101, Counter: counter},
			tx:      testutils.NewTransferFC(t, nil),
			wantErr: ErrBillLocked,
		},
		{
			name:    "BillData is nil",
			bd:      nil,
			tx:      testutils.NewTransferFC(t, nil),
			wantErr: ErrBillNil,
		},
		{
			name:    "TargetSystemIdentifier is zero",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetSystemID(0))),
			wantErr: ErrTargetSystemIdentifierEmpty,
		},
		{
			name:    "TargetRecordID is nil",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID(nil))),
			wantErr: ErrTargetRecordIDEmpty,
		},
		{
			name:    "TargetRecordID is empty",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID([]byte{}))),
			wantErr: ErrTargetRecordIDEmpty,
		},
		{
			name: "AdditionTime invalid",
			bd:   newBillData(101, counter),
			tx: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(
				testutils.WithEarliestAdditionTime(2),
				testutils.WithLatestAdditionTime(1),
			)),
			wantErr: ErrAdditionTimeInvalid,
		},
		{
			name:    "Invalid amount",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(102))),
			wantErr: ErrInvalidFCValue,
		},
		{
			name:    "Invalid fee",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(1))),
			wantErr: ErrInvalidFeeValue,
		},
		{
			name:    "Invalid counter",
			bd:      newBillData(101, counter),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithCounter(counter+10))),
			wantErr: ErrInvalidCounter,
		},
		{
			name: "RecordID exists",
			bd:   newBillData(101, counter),
			tx: testutils.NewTransferFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(101, counter),
			tx: testutils.NewTransferFC(t, nil,
				testtransaction.WithFeeProof([]byte{0, 0, 0, 0}),
			),
			wantErr: ErrFeeProofExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			attr := &fc.TransferFeeCreditAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := validateTransferFC(tt.tx, attr, tt.bd)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestReclaimFC(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	verifiers := map[string]abcrypto.Verifier{"test": verifier}

	var (
		amount  = uint64(100)
		counter = uint64(4)
	)

	tests := []struct {
		name       string
		bd         *money.BillData
		tx         *types.TransactionOrder
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			bd:      newBillData(amount, counter),
			tx:      testutils.NewReclaimFC(t, signer, nil),
			wantErr: nil,
		},
		{
			name:    "BillData is nil",
			bd:      nil,
			tx:      testutils.NewReclaimFC(t, signer, nil),
			wantErr: ErrBillNil,
		},
		{
			name: "Fee credit record exists",
			bd:   newBillData(amount, counter),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(amount, counter),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithFeeProof([]byte{0, 0, 0, 0}),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name: "Invalid target unit",
			bd:   newBillData(amount, counter),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithUnitID(money.NewFeeCreditRecordID(nil, []byte{2})),
			),
			wantErr: ErrReclaimFCInvalidTargetUnit,
		},
		{
			name: "Invalid tx fee",
			bd:   newBillData(amount, counter),
			tx: testutils.NewReclaimFC(t, signer,
				testutils.NewReclaimFCAttr(t, signer,
					testutils.WithReclaimFCClosureTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewCloseFC(t,
								testutils.NewCloseFCAttr(
									testutils.WithCloseFCAmount(2),
									testutils.WithCloseFCTargetUnitCounter(counter),
								),
							),
							ServerMetadata: &types.ServerMetadata{ActualFee: 10},
						},
					),
				),
			),
			wantErr: ErrReclaimFCInvalidTxFee,
		},
		{
			name:    "Invalid target unit counter",
			bd:      newBillData(amount, counter+1),
			tx:      testutils.NewReclaimFC(t, signer, nil),
			wantErr: ErrReclaimFCInvalidTargetUnitCounter,
		},
		{
			name:    "Invalid counter",
			bd:      newBillData(amount, counter),
			tx:      testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer, testutils.WithReclaimFCCounter(counter+1))),
			wantErr: ErrInvalidCounter,
		},
		{
			name: "Invalid proof",
			bd:   newBillData(amount, counter),
			tx: testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer,
				testutils.WithReclaimFCClosureProof(newInvalidProof(t, signer)),
			)),
			wantErrMsg: "invalid proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &fc.ReclaimFeeCreditAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			err := validateReclaimFC(tt.tx, attr, tt.bd, verifiers, crypto.SHA256)
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

func TestLockTx(t *testing.T) {
	counter := uint64(5)
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.LockAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   &money.BillData{Counter: counter, Locked: 0},
			attr: &money.LockAttributes{Counter: counter, LockStatus: 1},
		},
		{
			name: "attr is nil",
			bd:   &money.BillData{Counter: counter},
			attr: nil,
			res:  ErrTxAttrNil,
		},
		{
			name: "bill data is nil",
			bd:   nil,
			attr: &money.LockAttributes{Counter: counter},
			res:  ErrBillNil,
		},
		{
			name: "bill is already locked",
			bd:   &money.BillData{Counter: counter, Locked: 1},
			attr: &money.LockAttributes{Counter: counter},
			res:  ErrBillLocked,
		},
		{
			name: "zero lock value",
			bd:   &money.BillData{Counter: counter, Locked: 0},
			attr: &money.LockAttributes{Counter: counter, LockStatus: 0},
			res:  ErrInvalidLockStatus,
		},
		{
			name: "invalid counter",
			bd:   &money.BillData{Counter: counter, Locked: 0},
			attr: &money.LockAttributes{Counter: counter + 1, LockStatus: 1},
			res:  ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLockTx(tt.attr, tt.bd)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestUnlockTx(t *testing.T) {
	counter := uint64(5)
	tests := []struct {
		name string
		bd   *money.BillData
		attr *money.UnlockAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   &money.BillData{Counter: counter, Locked: 1},
			attr: &money.UnlockAttributes{Counter: counter},
		},
		{
			name: "attr is nil",
			bd:   &money.BillData{Counter: counter, Locked: 1},
			attr: nil,
			res:  ErrTxAttrNil,
		},
		{
			name: "bill data is nil",
			bd:   nil,
			attr: &money.UnlockAttributes{Counter: counter},
			res:  ErrBillNil,
		},
		{
			name: "bill is already unlocked",
			bd:   &money.BillData{Counter: counter, Locked: 0},
			attr: &money.UnlockAttributes{Counter: counter},
			res:  ErrBillUnlocked,
		},
		{
			name: "invalid counter",
			bd:   &money.BillData{Counter: counter, Locked: 1},
			attr: &money.UnlockAttributes{Counter: counter + 1},
			res:  ErrInvalidCounter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUnlockTx(tt.attr, tt.bd)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func newInvalidTargetValueSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        90,
		Counter:      6,
	})
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      []*types.TransactionRecord{transferDCRecord},
			DcTransferProofs: []*types.TxProof{nil},
			TargetValue:      100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newInvalidTargetUnitIDSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: []byte{0},
		Value:        100,
		Counter:      6,
	})
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      []*types.TransactionRecord{transferDCRecord},
			DcTransferProofs: []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
			TargetValue:      100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newInvalidTargetCounterSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferID := newBillID(1)
	swapID := newBillID(255)
	return createSwapDCTransactionOrder(t, signer, swapID, createTransferDCTransactionRecord(t, transferID, &money.TransferDCAttributes{
		TargetUnitID:      swapID,
		Value:             100,
		Counter:           6,
		TargetUnitCounter: 7,
	}))
}

func newDescBillOrderSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	// create swap tx with two dust transfers in descending order of bill ids
	billIds := []types.UnitID{newBillID(2), newBillID(1)}
	swapId := newBillID(255)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		transferIds[i] = billIds[i]
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}

	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      dcTransfers,
			DcTransferProofs: proofs,
			TargetValue:      200,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newEqualBillIdsSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	// create swap tx with two dust transfers with equal bill ids
	billIds := []types.UnitID{newBillID(1), newBillID(1)}
	swapId := newBillID(255)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		transferIds[i] = billIds[i]
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      dcTransfers,
			DcTransferProofs: proofs,
			TargetValue:      200,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newSwapOrderWithInvalidTargetSystemID(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID(0),
		testtransaction.WithPayloadType(money.PayloadTypeTransDC),
		testtransaction.WithUnitID(transferId),
		testtransaction.WithAttributes(&money.TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Counter:      6,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return createSwapDCTransactionOrder(t, signer, swapId, transferDCRecord)
}

func newDcProofsNilSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      []*types.TransactionRecord{transferDCRecord},
			DcTransferProofs: nil,
			TargetValue:      100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newEmptyDcProofsSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      6,
	})
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      []*types.TransactionRecord{transferDCRecord},
			DcTransferProofs: []*types.TxProof{{BlockHeaderHash: []byte{0}}},
			TargetValue:      100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newInvalidDcProofsSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	UCSigner, _ := testsig.CreateSignerAndVerifier(t)
	txo := newSwapDC(t, UCSigner)
	// newSwapDC uses passed in signer for both trustbase and txo
	// so we need to reset owner proof to the correct tx signer
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func newSwapDC(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := []byte{255}

	return createSwapDCTransactionOrder(t, signer, swapId, createTransferDCTransactionRecord(t, transferId, &money.TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Counter:      0,
	}))
}

func createSwapDCTransactionOrder(t *testing.T, signer abcrypto.Signer, swapId []byte, transferDCRecords ...*types.TransactionRecord) *types.TransactionOrder {
	var proofs []*types.TxProof
	for _, dcTx := range transferDCRecords {
		proofs = append(proofs, testblock.CreateProof(t, dcTx, signer))
	}
	txo := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithUnitID(swapId),
		testtransaction.WithAttributes(&money.SwapDCAttributes{
			OwnerCondition:   templates.AlwaysTrueBytes(),
			DcTransfers:      transferDCRecords,
			DcTransferProofs: proofs,
			TargetValue:      100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	require.NoError(t, txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer)))
	return txo
}

func createTransferDCTransactionRecord(t *testing.T, transferID []byte, attr *money.TransferDCAttributes) *types.TransactionRecord {
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(money.PayloadTypeTransDC),
		testtransaction.WithUnitID(transferID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return transferDCRecord
}

func newBillData(v uint64, counter uint64) *money.BillData {
	return &money.BillData{V: v, Counter: counter}
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	attr := testutils.NewDefaultReclaimFCAttr(t, signer)
	attr.CloseFeeCreditProof.BlockHeaderHash = []byte("invalid hash")
	return attr.CloseFeeCreditProof
}

type (
	stateMock struct {
		units map[string]*state.Unit
	}
	swapValidationOption func(c *swapValidationContext)
)

func withSwapStateUnit(unitID string, unit *state.Unit) swapValidationOption {
	return func(c *swapValidationContext) {
		s := c.state.(*stateMock)
		s.units[unitID] = unit
		if unit == nil {
			delete(s.units, unitID)
		}
	}
}

func newSwapValidationContext(t *testing.T, verifier abcrypto.Verifier, tx *types.TransactionOrder, opts ...swapValidationOption) *swapValidationContext {
	c := defaultSwapValidationContext(t, verifier, tx)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func defaultSwapValidationContext(t *testing.T, verifier abcrypto.Verifier, tx *types.TransactionOrder) *swapValidationContext {
	trustBase := map[string]abcrypto.Verifier{"test": verifier}
	attr := &money.SwapDCAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))

	// NB! using the same pubkey for trustbase and unit bearer! TODO: use different keys...
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	unit := state.NewUnit(templates.NewP2pkh256BytesFromKey(pubKey), &money.BillData{
		V:       0,
		T:       0,
		Counter: 0,
	})
	dcMoneySupplyUnit := state.NewUnit(nil, &money.BillData{
		V:       1e8,
		T:       0,
		Counter: 0,
	})
	s := &stateMock{units: map[string]*state.Unit{
		string(tx.UnitID()):                unit,
		string(DustCollectorMoneySupplyID): dcMoneySupplyUnit,
	}}
	return &swapValidationContext{
		tx:            tx,
		attr:          attr,
		state:         s,
		systemID:      1,
		hashAlgorithm: crypto.SHA256,
		trustBase:     trustBase,
		execPredicate: func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) error {
			return nil
		},
	}
}

func (s *stateMock) GetUnit(id types.UnitID, _ bool) (*state.Unit, error) {
	if s.units == nil {
		return nil, nil
	}
	unit, ok := s.units[string(id)]
	if !ok {
		return nil, errors.New("unit does not exist")
	}
	return unit, nil
}
