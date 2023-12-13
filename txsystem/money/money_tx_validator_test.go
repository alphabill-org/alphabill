package money

import (
	"crypto"
	"errors"
	"math"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		bd   *BillData
		attr *TransferAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferAttributes{TargetValue: 100, Backlink: []byte{6}},
			res:  nil,
		},
		{
			name: "LockedBill",
			bd:   &BillData{Locked: 1, V: 100, Backlink: []byte{6}},
			attr: &TransferAttributes{TargetValue: 100, Backlink: []byte{6}},
			res:  ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferAttributes{TargetValue: 101, Backlink: []byte{6}},
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferAttributes{TargetValue: 100, Backlink: []byte{5}},
			res:  ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransfer(tt.bd, tt.attr)
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
		bd   *BillData
		attr *TransferDCAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Backlink:     []byte{6},
			},
			res: nil,
		},
		{
			name: "LockedBill",
			bd:   &BillData{Locked: 1, V: 100, Backlink: []byte{6}},
			attr: &TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Backlink:     []byte{6},
			},
			res: ErrBillLocked,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        101,
				Backlink:     []byte{6},
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferDCAttributes{
				TargetUnitID: test.RandomBytes(32),
				Value:        100,
				Backlink:     test.RandomBytes(32),
			},
			res: ErrInvalidBacklink,
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
		bd   *BillData
		attr *SplitAttributes
		err  string
	}{
		{
			name: "Ok 2-way split",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Backlink:       []byte{6},
			},
		},
		{
			name: "Ok 3-way split",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 80,
				Backlink:       []byte{6},
			},
		},
		{
			name: "BillLocked",
			bd:   &BillData{Locked: 1, V: 100, Backlink: []byte{6}},
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Backlink:       []byte{6},
			},
			err: "bill is already locked",
		},
		{
			name: "Invalid backlink",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 50,
				Backlink:       []byte{5},
			},
			err: "the transaction backlink 0x05 is not equal to unit backlink 0x06",
		},
		{
			name: "Target units are empty",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits:    []*TargetUnit{},
				RemainingValue: 1,
				Backlink:       []byte{6},
			},
			err: "target units are empty",
		},
		{
			name: "Target unit is nil",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits:    []*TargetUnit{nil},
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			err: "target unit is nil",
		},
		{
			name: "Target unit amount is zero",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 0, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			err: "target unit amount is zero at index 0",
		},
		{
			name: "Target unit owner condition is empty",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 1, OwnerCondition: []byte{}},
				},
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			err: "target unit owner condition is empty at index 0",
		},
		{
			name: "Target unit amount overflow",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: math.MaxUint64, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 1, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			err: "failed to add target unit amounts",
		},
		{
			name: "Remaining value is zero",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 50, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 0,
				Backlink:       []byte{6},
			},
			err: "remaining value is zero",
		},
		{
			name: "Amount plus remaining value is less than bill value",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 79,
				Backlink:       []byte{6},
			},
			err: "the sum of the values to be transferred plus the remaining value must equal the value of the bill",
		},
		{
			name: "Amount plus remaining value is greater than bill value",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				TargetUnits: []*TargetUnit{
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
					{Amount: 10, OwnerCondition: templates.AlwaysTrueBytes()},
				},
				RemainingValue: 81,
				Backlink:       []byte{6},
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
				withSwapStateUnit(string(dustCollectorMoneySupplyID), state.NewUnit(nil, &BillData{
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
			ctx:  newSwapValidationContext(t, verifier, newInvalidTargetValueSwap(t)),
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
			name: "invalid target backlink",
			ctx:  newSwapValidationContext(t, verifier, newInvalidTargetBacklinkSwap(t, signer)),
			err:  "dust transfer target backlink is not equal to target unit backlink",
		},
		{
			name: "InvalidProofsNil",
			ctx:  newSwapValidationContext(t, verifier, newDcProofsNilSwap(t)),
			err:  "invalid count of proofs",
		},
		{
			name: "InvalidEmptyDcProof",
			ctx:  newSwapValidationContext(t, verifier, newEmptyDcProofsSwap(t)),
			err:  "unicity certificate is nil",
		},
		{
			name: "InvalidDcProofInvalid",
			ctx:  newSwapValidationContext(t, verifier, newInvalidDcProofsSwap(t)),
			err:  "invalid unicity seal signature",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ctx.validateSwapTx()
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

func TestTransferFC(t *testing.T) {
	backlink := []byte{4}
	tests := []struct {
		name    string
		bd      *BillData
		tx      *types.TransactionOrder
		wantErr error
	}{
		{
			name:    "Ok",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, nil),
			wantErr: nil,
		},
		{
			name:    "LockedBill",
			bd:      &BillData{Locked: 1, V: 101, Backlink: backlink},
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
			name:    "TargetSystemIdentifier is nil",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetSystemID(nil))),
			wantErr: ErrTargetSystemIdentifierEmpty,
		},
		{
			name:    "TargetSystemIdentifier is empty",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetSystemID([]byte{}))),
			wantErr: ErrTargetSystemIdentifierEmpty,
		},
		{
			name:    "TargetRecordID is nil",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID(nil))),
			wantErr: ErrTargetRecordIDEmpty,
		},
		{
			name:    "TargetRecordID is empty",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithTargetRecordID([]byte{}))),
			wantErr: ErrTargetRecordIDEmpty,
		},
		{
			name: "AdditionTime invalid",
			bd:   newBillData(101, backlink),
			tx: testutils.NewTransferFC(t, testutils.NewTransferFCAttr(
				testutils.WithEarliestAdditionTime(2),
				testutils.WithLatestAdditionTime(1),
			)),
			wantErr: ErrAdditionTimeInvalid,
		},
		{
			name:    "Invalid amount",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(102))),
			wantErr: ErrInvalidFCValue,
		},
		{
			name:    "Invalid fee",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithAmount(1))),
			wantErr: ErrInvalidFeeValue,
		},
		{
			name:    "Invalid backlink",
			bd:      newBillData(101, backlink),
			tx:      testutils.NewTransferFC(t, testutils.NewTransferFCAttr(testutils.WithBacklink([]byte("not backlink")))),
			wantErr: ErrInvalidBacklink,
		},
		{
			name: "RecordID exists",
			bd:   newBillData(101, backlink),
			tx: testutils.NewTransferFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(101, backlink),
			tx: testutils.NewTransferFC(t, nil,
				testtransaction.WithFeeProof([]byte{0, 0, 0, 0}),
			),
			wantErr: ErrFeeProofExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			attr := &transactions.TransferFeeCreditAttributes{}
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
		amount   = uint64(100)
		backlink = []byte{4}
	)

	tests := []struct {
		name       string
		bd         *BillData
		tx         *types.TransactionOrder
		wantErr    error
		wantErrMsg string
	}{
		{
			name:    "Ok",
			bd:      newBillData(amount, backlink),
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
			bd:   newBillData(amount, backlink),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(amount, backlink),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithFeeProof([]byte{0, 0, 0, 0}),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name: "Invalid target unit",
			bd:   newBillData(amount, backlink),
			tx: testutils.NewReclaimFC(t, signer, nil,
				testtransaction.WithUnitId(NewFeeCreditRecordID(nil, []byte{2})),
			),
			wantErr: ErrReclaimFCInvalidTargetUnit,
		},
		{
			name: "Invalid tx fee",
			bd:   newBillData(amount, backlink),
			tx: testutils.NewReclaimFC(t, signer,
				testutils.NewReclaimFCAttr(t, signer,
					testutils.WithReclaimFCClosureTx(
						&types.TransactionRecord{
							TransactionOrder: testutils.NewCloseFC(t,
								testutils.NewCloseFCAttr(
									testutils.WithCloseFCAmount(2),
									testutils.WithCloseFCTargetUnitBacklink(backlink),
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
			name:    "Invalid target unit backlink",
			bd:      newBillData(amount, []byte("target unit backlink not equal to bill backlink")),
			tx:      testutils.NewReclaimFC(t, signer, nil),
			wantErr: ErrReclaimFCInvalidTargetUnitBacklink,
		},
		{
			name:    "Invalid backlink",
			bd:      newBillData(amount, backlink),
			tx:      testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer, testutils.WithReclaimFCBacklink([]byte("backlink not equal")))),
			wantErr: ErrInvalidBacklink,
		},
		{
			name: "Invalid proof",
			bd:   newBillData(amount, backlink),
			tx: testutils.NewReclaimFC(t, signer, testutils.NewReclaimFCAttr(t, signer,
				testutils.WithReclaimFCClosureProof(newInvalidProof(t, signer)),
			)),
			wantErrMsg: "invalid proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &transactions.ReclaimFeeCreditAttributes{}
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
	tests := []struct {
		name string
		bd   *BillData
		attr *LockAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   &BillData{Backlink: []byte{5}, Locked: 0},
			attr: &LockAttributes{Backlink: []byte{5}, LockStatus: 1},
		},
		{
			name: "attr is nil",
			bd:   &BillData{Backlink: []byte{5}},
			attr: nil,
			res:  ErrTxAttrNil,
		},
		{
			name: "bill data is nil",
			bd:   nil,
			attr: &LockAttributes{Backlink: []byte{5}},
			res:  ErrBillNil,
		},
		{
			name: "bill is already locked",
			bd:   &BillData{Backlink: []byte{5}, Locked: 1},
			attr: &LockAttributes{Backlink: []byte{5}},
			res:  ErrBillLocked,
		},
		{
			name: "zero lock value",
			bd:   &BillData{Backlink: []byte{5}, Locked: 0},
			attr: &LockAttributes{Backlink: []byte{5}, LockStatus: 0},
			res:  ErrInvalidLockStatus,
		},
		{
			name: "invalid backlink",
			bd:   &BillData{Backlink: []byte{5}, Locked: 0},
			attr: &LockAttributes{Backlink: []byte{6}, LockStatus: 1},
			res:  ErrInvalidBacklink,
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
	tests := []struct {
		name string
		bd   *BillData
		attr *UnlockAttributes
		res  error
	}{
		{
			name: "Ok",
			bd:   &BillData{Backlink: []byte{5}, Locked: 1},
			attr: &UnlockAttributes{Backlink: []byte{5}},
		},
		{
			name: "attr is nil",
			bd:   &BillData{Backlink: []byte{5}, Locked: 1},
			attr: nil,
			res:  ErrTxAttrNil,
		},
		{
			name: "bill data is nil",
			bd:   nil,
			attr: &UnlockAttributes{Backlink: []byte{5}},
			res:  ErrBillNil,
		},
		{
			name: "bill is already unlocked",
			bd:   &BillData{Backlink: []byte{5}, Locked: 0},
			attr: &UnlockAttributes{Backlink: []byte{5}},
			res:  ErrBillUnlocked,
		},
		{
			name: "invalid backlink",
			bd:   &BillData{Backlink: []byte{5}, Locked: 1},
			attr: &UnlockAttributes{Backlink: []byte{6}},
			res:  ErrInvalidBacklink,
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

func newInvalidTargetValueSwap(t *testing.T) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        90,
		Backlink:     []byte{6},
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func newInvalidTargetUnitIDSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID: []byte{0},
		Value:        100,
		Backlink:     []byte{6},
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func newInvalidTargetBacklinkSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)
	return createSwapDCTransactionOrder(t, signer, swapId, createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID:       swapId,
		Value:              100,
		Backlink:           []byte{6},
		TargetUnitBacklink: []byte{7},
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
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Backlink:     []byte{6},
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}

	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
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
		dcTransfers[i] = createTransferDCTransactionRecord(t, billIds[i], &TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Backlink:     []byte{6},
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func newSwapOrderWithInvalidTargetSystemID(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID([]byte{0, 0, 0, 1}),
		testtransaction.WithPayloadType(PayloadTypeTransDC),
		testtransaction.WithUnitId(transferId),
		testtransaction.WithAttributes(&TransferDCAttributes{
			TargetUnitID: swapId,
			Value:        100,
			Backlink:     []byte{6},
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return createSwapDCTransactionOrder(t, signer, swapId, transferDCRecord)
}

func newDcProofsNilSwap(t *testing.T) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID: swapId,

		Value:    100,
		Backlink: []byte{6},
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func newEmptyDcProofsSwap(t *testing.T) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := newBillID(255)
	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Backlink:     []byte{6},
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func newInvalidDcProofsSwap(t *testing.T) *types.TransactionOrder {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	return newSwapDC(t, signer)
}

func newSwapDC(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	transferId := newBillID(1)
	swapId := []byte{255}

	return createSwapDCTransactionOrder(t, signer, swapId, createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		TargetUnitID: swapId,
		Value:        100,
		Backlink:     []byte{6},
	}))
}

func createSwapDCTransactionOrder(t *testing.T, signer abcrypto.Signer, swapId []byte, transferDCRecords ...*types.TransactionRecord) *types.TransactionOrder {
	var proofs []*types.TxProof
	for _, dcTx := range transferDCRecords {
		proofs = append(proofs, testblock.CreateProof(t, dcTx, signer))
	}
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
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
		testtransaction.WithOwnerProof(nil),
	)
}

func createTransferDCTransactionRecord(t *testing.T, transferID []byte, attr *TransferDCAttributes) *types.TransactionRecord {
	transferDCRecord := testtransaction.NewTransactionRecord(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeTransDC),
		testtransaction.WithUnitId(transferID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
	)
	return transferDCRecord
}

func newBillData(v uint64, backlink []byte) *BillData {
	return &BillData{V: v, Backlink: backlink}
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
	attr := &SwapDCAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))

	unit := state.NewUnit(nil, &BillData{
		V:        0,
		T:        0,
		Backlink: nil,
	})
	dcMoneySupplyUnit := state.NewUnit(nil, &BillData{
		V:        1e8,
		T:        0,
		Backlink: nil,
	})
	s := &stateMock{units: map[string]*state.Unit{
		string(tx.UnitID()):                unit,
		string(dustCollectorMoneySupplyID): dcMoneySupplyUnit,
	}}
	return &swapValidationContext{
		tx:            tx,
		attr:          attr,
		state:         s,
		systemID:      []byte{0, 0, 0, 0},
		hashAlgorithm: crypto.SHA256,
		trustBase:     trustBase,
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
