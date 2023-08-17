package money

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const swapTimeout = 20

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
				Nonce:        test.RandomBytes(32),
				TargetBearer: script.PredicateAlwaysTrue(),
				TargetValue:  100,
				Backlink:     []byte{6},
			},
			res: nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferDCAttributes{
				Nonce:        test.RandomBytes(32),
				TargetBearer: script.PredicateAlwaysTrue(),
				TargetValue:  101,
				Backlink:     []byte{6},
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			attr: &TransferDCAttributes{
				Nonce:        test.RandomBytes(32),
				TargetBearer: script.PredicateAlwaysTrue(),
				TargetValue:  100,
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
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         50,
				RemainingValue: 50,
				Backlink:       []byte{6},
			},
			res: nil,
		},
		{
			name: "AmountExceedsBillValue",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         101,
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "AmountEqualsBillValue",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         100,
				RemainingValue: 0,
				Backlink:       []byte{6},
			},
			res: ErrSplitBillZeroRemainder,
		},
		{
			name: "Amount is zero (0:100)",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         0,
				RemainingValue: 100,
				Backlink:       []byte{6},
			},
			res: ErrSplitBillZeroAmount,
		},
		{
			name: "Amount is zero (0:30)",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         0,
				RemainingValue: 30,
				Backlink:       []byte{6},
			},
			res: ErrSplitBillZeroAmount,
		},
		{
			name: "InvalidRemainingValue - zero remaining (50:0)",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         50,
				RemainingValue: 0,
				Backlink:       []byte{6},
			},
			res: ErrSplitBillZeroRemainder,
		},
		{
			name: "InvalidRemainingValue - smaller than amount",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         50,
				RemainingValue: 49,
				Backlink:       []byte{6},
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidRemainingValue - greater than amount",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         50,
				RemainingValue: 51,
				Backlink:       []byte{6},
			},
			res: ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			attr: &SplitAttributes{
				Amount:         50,
				RemainingValue: 50,
				Backlink:       []byte{5},
			},
			res: ErrInvalidBacklink,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplit(tt.bd, tt.attr)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestSwap(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	tests := []struct {
		name    string
		tx      *types.TransactionOrder
		blockNo uint64
		err     string
	}{
		{
			name:    "Ok",
			tx:      newSwapDC(t, signer),
			blockNo: 10,
		},
		{
			name:    "SwapTimeout_LessThanCurrentBlock_NOK",
			tx:      newSwapWithTimeout(t, signer, 9),
			blockNo: 10,
			err:     "dust transfer timeout exceeded",
		},
		{
			name:    "SwapTimeout_EqualToCurrentBlock_NOK",
			tx:      newSwapWithTimeout(t, signer, 10),
			blockNo: 10,
			err:     "dust transfer timeout exceeded",
		},
		{
			name:    "SwapTimeout_GreaterThanCurrentBlock_OK",
			tx:      newSwapWithTimeout(t, signer, 11),
			blockNo: 10,
		},
		{
			name:    "SwapTimeout_DustTransferTimeoutsNotEqual",
			tx:      newSwapWithInvalidDustTimeouts(t, signer),
			blockNo: 10,
			err:     "dust transfer timeouts are not equal",
		},
		{
			name:    "InvalidTargetValue",
			tx:      newInvalidTargetValueSwap(t),
			blockNo: 10,
			err:     ErrSwapInvalidTargetValue.Error(),
		},
		{
			name:    "InvalidBillIdentifiers",
			tx:      newInvalidBillIdentifierSwap(t, signer),
			blockNo: 10,
			err:     ErrSwapInvalidBillIdentifiers.Error(),
		},
		{
			name:    "InvalidBillId",
			tx:      newInvalidBillIdSwap(t, signer),
			blockNo: 10,
			err:     ErrSwapInvalidBillId.Error(),
		},
		{
			name:    "DustTransfersInDescBillIdOrder",
			tx:      newSwapWithDescBillOrder(t, signer),
			blockNo: 10,
			err:     ErrSwapDustTransfersInvalidOrder.Error(),
		},
		{
			name:    "DustTransfersInEqualBillIdOrder",
			tx:      newSwapOrderWithEqualBillIds(t, signer),
			blockNo: 10,
			err:     ErrSwapDustTransfersInvalidOrder.Error(),
		},
		{
			name:    "InvalidNonce",
			tx:      newInvalidNonceSwap(t, signer),
			blockNo: 10,
			err:     ErrSwapInvalidNonce.Error(),
		},
		{
			name:    "InvalidTargetBearer",
			tx:      newInvalidTargetBearerSwap(t, signer),
			blockNo: 10,
			err:     ErrSwapInvalidTargetBearer.Error(),
		},
		{
			name:    "InvalidProofsNil",
			tx:      newDcProofsNilSwap(t),
			blockNo: 10,
			err:     "invalid count of proofs",
		},
		{
			name:    "InvalidEmptyDcProof",
			tx:      newEmptyDcProofsSwap(t),
			blockNo: 10,
			err:     "unicity certificate is nil",
		},
		{
			name:    "InvalidDcProofInvalid",
			tx:      newInvalidDcProofsSwap(t),
			blockNo: 10,
			err:     "unicity seal validation failed, signature verification error",
		},
		{
			name:    "InvalidSwapOwnerProof",
			tx:      newSwapOrderWithWrongOwnerCondition(t, signer),
			blockNo: 10,
			err:     ErrSwapOwnerProofFailed.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trustBase := map[string]abcrypto.Verifier{"test": verifier}
			attr := &SwapDCAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			dustTransfers, err := getDCTransfers(attr)
			require.NoError(t, err)
			err = validateSwap(tt.tx, attr, dustTransfers, tt.blockNo, crypto.SHA256, trustBase)
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
			tx:      testfc.NewTransferFC(t, nil),
			wantErr: nil,
		},
		{
			name:    "BillData is nil",
			bd:      nil,
			tx:      testfc.NewTransferFC(t, nil),
			wantErr: ErrBillNil,
		},
		{
			name:    "TargetSystemIdentifier is nil",
			bd:      newBillData(101, backlink),
			tx:      testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetSystemID(nil))),
			wantErr: ErrTargetSystemIdentifierNil,
		},
		{
			name:    "TargetRecordID is nil",
			bd:      newBillData(101, backlink),
			tx:      testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithTargetRecordID(nil))),
			wantErr: ErrTargetRecordIDNil,
		},
		{
			name:    "AdditionTime invalid",
			bd:      newBillData(101, backlink),
			tx:      testfc.NewTransferFC(t, testfc.NewTransferFCAttr(
				testfc.WithEarliestAdditionTime(2),
				testfc.WithLatestAdditionTime(1),
			)),
			wantErr: ErrAdditionTimeInvalid,
		},
		{
			name:    "Invalid amount",
			bd:      newBillData(101, backlink),
			tx:      testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithAmount(101))),
			wantErr: ErrInvalidFCValue,
		},
		{
			name:    "Invalid backlink",
			bd:      newBillData(101, backlink),
			tx:      testfc.NewTransferFC(t, testfc.NewTransferFCAttr(testfc.WithBacklink([]byte("not backlink")))),
			wantErr: ErrInvalidBacklink,
		},
		{
			name: "RecordID exists",
			bd:   newBillData(101, backlink),
			tx: testfc.NewTransferFC(t, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(101, backlink),
			tx: testfc.NewTransferFC(t, nil,
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
			tx:      testfc.NewReclaimFC(t, signer, nil),
			wantErr: nil,
		},
		{
			name:    "BillData is nil",
			bd:      nil,
			tx:      testfc.NewReclaimFC(t, signer, nil),
			wantErr: ErrBillNil,
		},
		{
			name: "Fee credit record exists",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{0}}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithFeeProof([]byte{0, 0, 0, 0}),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name: "Invalid target unit",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithUnitId(test.NewUnitID(2)),
			),
			wantErr: ErrReclaimFCInvalidTargetUnit,
		},
		{
			name: "Invalid tx fee",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer,
				testfc.NewReclaimFCAttr(t, signer,
					testfc.WithReclaimFCClosureTx(
						&types.TransactionRecord{
							TransactionOrder: testfc.NewCloseFC(t,
								testfc.NewCloseFCAttr(
									testfc.WithCloseFCAmount(2),
									testfc.WithCloseFCNonce(backlink),
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
			name:    "Invalid nonce",
			bd:      newBillData(amount, []byte("nonce not equal to bill backlink")),
			tx:      testfc.NewReclaimFC(t, signer, nil),
			wantErr: ErrReclaimFCInvalidNonce,
		},
		{
			name:    "Invalid backlink",
			bd:      newBillData(amount, backlink),
			tx:      testfc.NewReclaimFC(t, signer, testfc.NewReclaimFCAttr(t, signer, testfc.WithReclaimFCBacklink([]byte("backlink not equal")))),
			wantErr: ErrInvalidBacklink,
		},
		{
			name: "Invalid proof",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, testfc.NewReclaimFCAttr(t, signer,
				testfc.WithReclaimFCClosureProof(newInvalidProof(t, signer)),
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

func newSwapWithTimeout(t *testing.T, signer abcrypto.Signer, swapTimeout uint64) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	return createSwapDCTransactionOrder(t, signer, swapId, transferId,
		createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
			Nonce:        swapId,
			TargetBearer: script.PredicateAlwaysTrue(),
			TargetValue:  100,
			Backlink:     []byte{6},
			SwapTimeout:  swapTimeout,
		}),
	)
}

func newSwapWithInvalidDustTimeouts(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTx1 := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  20,
	})
	dcTx2 := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  21,
	})
	return createSwapDCTransactionOrder(t, signer, swapId, transferId,
		dcTx1, dcTx2,
	)
}

func newInvalidTargetValueSwap(t *testing.T) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  90,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{nil},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newInvalidBillIdentifierSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{swapId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newInvalidBillIdSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	// create swap tx with two dust transfers in descending order of bill ids
	billIds := []*uint256.Int{uint256.NewInt(1), uint256.NewInt(2)}
	swapId := calculateSwapID(billIds...)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		bytes32 := billIds[i].Bytes32()
		transferIds[i] = bytes32[:]
		dcTransfers[i] = createTransferDCTransactionRecord(t, bytes32[:], &TransferDCAttributes{
			Nonce:        swapId,
			TargetBearer: script.PredicateAlwaysTrue(),
			TargetValue:  100,
			Backlink:     []byte{6},
			SwapTimeout:  swapTimeout,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}

	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(transferIds[0]),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: transferIds,
			DcTransfers:     dcTransfers,
			Proofs:          proofs,
			TargetValue:     200,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newInvalidNonceSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        []byte{0},
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newSwapWithDescBillOrder(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	// create swap tx with two dust transfers in descending order of bill ids
	billIds := []*uint256.Int{uint256.NewInt(2), uint256.NewInt(1)}
	swapId := calculateSwapID(billIds...)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		bytes32 := billIds[i].Bytes32()
		transferIds[i] = bytes32[:]
		dcTransfers[i] = createTransferDCTransactionRecord(t, bytes32[:], &TransferDCAttributes{
			Nonce:        swapId,
			TargetBearer: script.PredicateAlwaysTrue(),
			TargetValue:  100,
			Backlink:     []byte{6},
			SwapTimeout:  swapTimeout,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}

	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: transferIds,
			DcTransfers:     dcTransfers,
			Proofs:          proofs,
			TargetValue:     200,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newSwapOrderWithEqualBillIds(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	// create swap tx with two dust transfers with equal bill ids
	billIds := []*uint256.Int{uint256.NewInt(1), uint256.NewInt(1)}
	swapId := calculateSwapID(billIds...)
	dcTransfers := make([]*types.TransactionRecord, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*types.TxProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		bytes32 := billIds[i].Bytes32()
		transferIds[i] = bytes32[:]
		dcTransfers[i] = createTransferDCTransactionRecord(t, bytes32[:], &TransferDCAttributes{
			Nonce:        swapId,
			TargetBearer: script.PredicateAlwaysTrue(),
			TargetValue:  100,
			Backlink:     []byte{6},
			SwapTimeout:  swapTimeout,
		})
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer)
	}
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: transferIds,
			DcTransfers:     dcTransfers,
			Proofs:          proofs,
			TargetValue:     200,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newSwapOrderWithWrongOwnerCondition(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateAlwaysFalse()),
	)
}

func newInvalidTargetBearerSwap(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysFalse(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{testblock.CreateProof(t, transferDCRecord, signer)},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newDcProofsNilSwap(t *testing.T) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          nil,
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)

}

func newEmptyDcProofsSwap(t *testing.T) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)

	transferDCRecord := createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
		Nonce:        swapId,
		TargetBearer: script.PredicateAlwaysTrue(),
		TargetValue:  100,
		Backlink:     []byte{6},
		SwapTimeout:  swapTimeout,
	})
	return testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithSystemID(systemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeSwapDC),
		testtransaction.WithUnitId(swapId),
		testtransaction.WithAttributes(&SwapDCAttributes{
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     []*types.TransactionRecord{transferDCRecord},
			Proofs:          []*types.TxProof{{BlockHeaderHash: []byte{0}}},
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
}

func newInvalidDcProofsSwap(t *testing.T) *types.TransactionOrder {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	return newSwapDC(t, signer)
}

func newSwapDC(t *testing.T, signer abcrypto.Signer) *types.TransactionOrder {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	return createSwapDCTransactionOrder(t, signer, swapId, transferId,
		createTransferDCTransactionRecord(t, transferId, &TransferDCAttributes{
			Nonce:        swapId,
			TargetBearer: script.PredicateAlwaysTrue(),
			TargetValue:  100,
			Backlink:     []byte{6},
			SwapTimeout:  swapTimeout,
		}),
	)
}

func createSwapDCTransactionOrder(t *testing.T, signer abcrypto.Signer, swapId []byte, transferId []byte, transferDCRecords ...*types.TransactionRecord) *types.TransactionOrder {
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
			OwnerCondition:  script.PredicateAlwaysTrue(),
			BillIdentifiers: [][]byte{transferId},
			DcTransfers:     transferDCRecords,
			Proofs:          proofs,
			TargetValue:     100,
		}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           100,
			MaxTransactionFee: 10,
			FeeCreditRecordID: []byte{0},
		}),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
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

func calculateSwapID(ids ...*uint256.Int) []byte {
	hasher := crypto.SHA256.New()
	for _, id := range ids {
		bytes32 := id.Bytes32()
		hasher.Write(bytes32[:])
	}
	return hasher.Sum(nil)
}

func newBillData(v uint64, backlink []byte) *BillData {
	return &BillData{V: v, Backlink: backlink}
}

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *types.TxProof {
	attr := testfc.NewDefaultReclaimFCAttr(t, signer)
	attr.CloseFeeCreditProof.BlockHeaderHash = []byte("invalid hash")
	return attr.CloseFeeCreditProof
}
