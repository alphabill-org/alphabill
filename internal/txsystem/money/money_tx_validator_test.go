package money

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		bd   *BillData
		tx   *transferWrapper
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(t, 100, []byte{6}),
			res:  nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(t, 101, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(t, 100, []byte{5}),
			res:  txsystem.ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransfer(tt.bd, tt.tx)
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
		tx   *transferDCWrapper
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(t, 100, []byte{6}, []byte{1}, test.RandomBytes(32)),
			res:  nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(t, 101, []byte{6}, []byte{1}, test.RandomBytes(32)),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(t, 100, []byte{5}, []byte{1}, test.RandomBytes(32)),
			res:  txsystem.ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransferDC(tt.bd, tt.tx)
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
		tx   *billSplitWrapper
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 50, 50, []byte{6}),
			res:  nil,
		},
		{
			name: "AmountExceedsBillValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 101, 100, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "AmountEqualsBillValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 100, 0, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidRemainingValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 50, 51, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 50, 50, []byte{5}),
			res:  txsystem.ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplit(tt.bd, tt.tx)
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
		name string
		tx   *swapWrapper
		err  string
	}{
		{
			name: "Ok",
			tx:   newValidSwap(t, signer),
			err:  "",
		},
		{
			name: "InvalidTargetValue",
			tx:   newInvalidTargetValueSwap(t),
			err:  ErrSwapInvalidTargetValue.Error(),
		},
		{
			name: "InvalidBillIdentifiers",
			tx:   newInvalidBillIdentifierSwap(t, signer),
			err:  ErrSwapInvalidBillIdentifiers.Error(),
		},
		{
			name: "InvalidBillId",
			tx:   newInvalidBillIdSwap(t, signer),
			err:  ErrSwapInvalidBillId.Error(),
		},
		{
			name: "DustTransfersInDescBillIdOrder",
			tx:   newSwapWithDescBillOrder(t, signer),
			err:  ErrSwapDustTransfersInvalidOrder.Error(),
		},
		{
			name: "DustTransfersInEqualBillIdOrder",
			tx:   newSwapOrderWithEqualBillIds(t, signer),
			err:  ErrSwapDustTransfersInvalidOrder.Error(),
		},
		{
			name: "InvalidNonce",
			tx:   newInvalidNonceSwap(t, signer),
			err:  ErrSwapInvalidNonce.Error(),
		},
		{
			name: "InvalidTargetBearer",
			tx:   newInvalidTargetBearerSwap(t, signer),
			err:  ErrSwapInvalidTargetBearer.Error(),
		},
		{
			name: "InvalidProofsNil",
			tx:   newDcProofsNilSwap(t),
			err:  "invalid count of proofs",
		},
		{
			name: "InvalidEmptyDcProof",
			tx:   newEmptyDcProofsSwap(t),
			err:  "unicity certificate is nil",
		},
		{
			name: "InvalidDcProofInvalid",
			tx:   newInvalidDcProofsSwap(t),
			err:  "invalid unicity seal signature",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trustBase := map[string]abcrypto.Verifier{"test": verifier}
			err := validateSwap(tt.tx, crypto.SHA256, trustBase)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

func TestTransferFC(t *testing.T) {
	tests := []struct {
		name    string
		bd      *BillData
		tx      *fc.TransferFeeCreditWrapper
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
			name:    "Tx is nil",
			bd:      newBillData(101, backlink),
			tx:      nil,
			wantErr: ErrTxNil,
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
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{FeeCreditRecordId: fcRecordID}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(101, backlink),
			tx: testfc.NewTransferFC(t, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErr: ErrFeeProofExists,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransferFC(tt.tx, tt.bd)
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

	tests := []struct {
		name       string
		bd         *BillData
		tx         *fc.ReclaimFeeCreditWrapper
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
			name:    "Tx is nil",
			bd:      newBillData(amount, backlink),
			tx:      nil,
			wantErr: ErrTxNil,
		},
		{
			name: "Fee credit record exists",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithClientMetadata(&txsystem.ClientMetadata{FeeCreditRecordId: recordID}),
			),
			wantErr: ErrRecordIDExists,
		},
		{
			name: "Fee proof exists",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithFeeProof(feeProof),
			),
			wantErr: ErrFeeProofExists,
		},
		{
			name: "Invalid target unit",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, nil,
				testtransaction.WithUnitId(test.NewUnitID(2)),
			),
			wantErr: ErrReclFCInvalidTargetUnit,
		},
		{
			name:    "Invalid tx fee",
			bd:      newBillData(1, backlink),
			tx:      testfc.NewReclaimFC(t, signer, nil),
			wantErr: ErrReclFCInvalidTxFee,
		},
		{
			name:    "Invalid nonce",
			bd:      newBillData(amount, []byte("nonce not equal to bill backlink")),
			tx:      testfc.NewReclaimFC(t, signer, nil),
			wantErr: ErrReclFCInvalidNonce,
		},
		{
			name:    "Invalid backlink",
			bd:      newBillData(amount, backlink),
			tx:      testfc.NewReclaimFC(t, signer, testfc.NewReclFCAttr(t, signer, testfc.WithReclFCBacklink([]byte("backlink not equal")))),
			wantErr: ErrInvalidBacklink,
		},
		{
			name: "Invalid proof type",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, testfc.NewReclFCAttr(t, signer,
				testfc.WithReclFCProof(&block.BlockProof{ProofType: block.ProofType_NOTRANS}),
			)),
			wantErr: ErrInvalidProofType,
		},
		{
			name: "Invalid proof",
			bd:   newBillData(amount, backlink),
			tx: testfc.NewReclaimFC(t, signer, testfc.NewReclFCAttr(t, signer,
				testfc.WithReclFCProof(newInvalidProof(t, signer)),
			)),
			wantErrMsg: "reclaimFC: invalid proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReclaimFC(tt.tx, tt.bd, verifiers, crypto.SHA256)
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

func newTransfer(t *testing.T, v uint64, backlink []byte) *transferWrapper {
	tx, err := NewMoneyTx(systemIdentifier, newPBTransactionOrder([]byte{1}, []byte{3}, 2, &TransferOrder{
		NewBearer:   []byte{4},
		TargetValue: v,
		Backlink:    backlink,
	}))
	require.NoError(t, err)
	require.IsType(t, tx, &transferWrapper{})
	return tx.(*transferWrapper)
}

func newTransferDC(t *testing.T, v uint64, backlink []byte, unitID []byte, nonce []byte) *transferDCWrapper {
	order := newPBTransactionOrder(unitID, []byte{3}, 2, &TransferDCOrder{
		Nonce:        nonce,
		TargetBearer: []byte{4},
		TargetValue:  v,
		Backlink:     backlink,
	})
	order.SystemId = systemIdentifier
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &transferDCWrapper{})
	return tx.(*transferDCWrapper)
}

func newSplit(t *testing.T, amount uint64, remainingValue uint64, backlink []byte) *billSplitWrapper {
	order := newPBTransactionOrder([]byte{1}, []byte{3}, 2, &SplitOrder{
		Amount:         amount,
		TargetBearer:   []byte{5},
		RemainingValue: remainingValue,
		Backlink:       backlink,
	})
	order.SystemId = systemIdentifier
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &billSplitWrapper{})
	return tx.(*billSplitWrapper)
}

func newInvalidTargetValueSwap(t *testing.T) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	order := newPBTransactionOrder(swapId, []byte{3}, 2, &SwapOrder{
		OwnerCondition:  dcTransfer.TargetBearer(),
		BillIdentifiers: [][]byte{transferId},
		DcTransfers:     []*txsystem.Transaction{dcTransfer.transaction},
		Proofs:          []*block.BlockProof{},
		TargetValue:     dcTransfer.TargetValue() - 1,
	})
	order.SystemId = systemIdentifier
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidBillIdentifierSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, test.RandomBytes(3), swapId)
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidBillIdSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder([]byte{0}, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	order.SystemId = systemIdentifier
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidNonceSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, []byte{0})
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}

	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newSwapWithDescBillOrder(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	// create swap tx with two dust transfers in descending order of bill ids
	billIds := []*uint256.Int{uint256.NewInt(2), uint256.NewInt(1)}
	swapId := calculateSwapID(billIds...)
	dcTransfers := make([]*transferDCWrapper, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*block.BlockProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		bytes32 := billIds[i].Bytes32()
		transferIds[i] = bytes32[:]
		dcTransfers[i] = newTransferDC(t, 100, []byte{6}, bytes32[:], swapId)
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer, bytes32[:])
	}
	swapTx := newSwapOrderWithDCTransfers([]byte{4}, 200, dcTransfers, transferIds, proofs)
	swapTxProto := newPBTransactionOrder(swapId, []byte{4}, 2, swapTx)
	tx, err := NewMoneyTx(systemIdentifier, swapTxProto)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newSwapOrderWithEqualBillIds(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	// create swap tx with two dust transfers with equal bill ids
	billIds := []*uint256.Int{uint256.NewInt(1), uint256.NewInt(1)}
	swapId := calculateSwapID(billIds...)
	dcTransfers := make([]*transferDCWrapper, len(billIds))
	transferIds := make([][]byte, len(billIds))
	proofs := make([]*block.BlockProof, len(billIds))
	for i := 0; i < len(billIds); i++ {
		bytes32 := billIds[i].Bytes32()
		transferIds[i] = bytes32[:]
		dcTransfers[i] = newTransferDC(t, 100, []byte{6}, bytes32[:], swapId)
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer, bytes32[:])
	}
	swapTx := newSwapOrderWithDCTransfers([]byte{4}, 200, dcTransfers, transferIds, proofs)
	swapTxProto := newPBTransactionOrder(swapId, []byte{4}, 2, swapTx)
	tx, err := NewMoneyTx(systemIdentifier, swapTxProto)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidTargetBearerSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	order := newPBTransactionOrder(swapId, []byte{3}, 2, &SwapOrder{
		OwnerCondition:  test.RandomBytes(32),
		BillIdentifiers: [][]byte{transferId},
		DcTransfers:     []*txsystem.Transaction{dcTransfer.transaction},
		Proofs:          []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])},
		TargetValue:     dcTransfer.TargetValue(),
	})
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newDcProofsNilSwap(t *testing.T) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, nil))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newEmptyDcProofsSwap(t *testing.T) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	proofs := []*block.BlockProof{&block.BlockProof{}}
	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidDcProofsSwap(t *testing.T) *swapWrapper {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newValidSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId)
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder(swapId, []byte{3}, 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newSwapOrder(dcTransfer *transferDCWrapper, transferDCID []byte, proof []*block.BlockProof) *SwapOrder {
	return &SwapOrder{
		OwnerCondition:  dcTransfer.TargetBearer(),
		BillIdentifiers: [][]byte{transferDCID},
		DcTransfers:     []*txsystem.Transaction{dcTransfer.transaction},
		Proofs:          proof,
		TargetValue:     dcTransfer.TargetValue(),
	}
}

func newSwapOrderWithDCTransfers(ownerCondition []byte, targetValue uint64, dcTransfers []*transferDCWrapper, transferDCIDs [][]byte, proofs []*block.BlockProof) *SwapOrder {
	wrappedDcTransfers := make([]*txsystem.Transaction, len(dcTransfers))
	for i, dcTransfer := range dcTransfers {
		wrappedDcTransfers[i] = dcTransfer.transaction
	}
	return &SwapOrder{
		OwnerCondition:  ownerCondition,
		BillIdentifiers: transferDCIDs,
		DcTransfers:     wrappedDcTransfers,
		Proofs:          proofs,
		TargetValue:     targetValue,
	}
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

func newInvalidProof(t *testing.T, signer abcrypto.Signer) *block.BlockProof {
	attr := testfc.NewDefaultReclFCAttr(t, signer)
	attr.CloseFeeCreditProof.TransactionsHash = []byte("invalid hash")
	return attr.CloseFeeCreditProof
}
