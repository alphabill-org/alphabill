package money

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/txsystem"
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
			tx:   newTransferDC(t, 100, []byte{6}, []byte{1}, test.RandomBytes(32), script.PredicateAlwaysTrue()),
			res:  nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(t, 101, []byte{6}, []byte{1}, test.RandomBytes(32), script.PredicateAlwaysTrue()),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(t, 100, []byte{5}, []byte{1}, test.RandomBytes(32), script.PredicateAlwaysTrue()),
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
			res:  ErrSplitBillZeroRemainder,
		},
		{
			name: "Amount is zero (0:100)",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 0, 100, []byte{6}),
			res:  ErrSplitBillZeroAmount,
		},
		{
			name: "Amount is zero (0:30)",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 0, 30, []byte{6}),
			res:  ErrSplitBillZeroAmount,
		},
		{
			name: "InvalidRemainingValue - zero remaining (50:0)",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 50, 0, []byte{6}),
			res:  ErrSplitBillZeroRemainder,
		},
		{
			name: "InvalidRemainingValue - smaller than amount",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(t, 50, 49, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidRemainingValue - greater than amount",
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
			err:  "unicity seal signature verification failed",
		},
		{
			name: "InvalidSwapOwnerProof",
			tx:   newSwapOrderWithWrongOwnerCondition(t, signer),
			err:  ErrSwapOwnerProofFailed.Error(),
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

func newTransferDC(t *testing.T, v uint64, backlink []byte, unitID []byte, nonce []byte, ownerProof []byte) *transferDCWrapper {
	order := newPBTransactionOrder(unitID, ownerProof, 2, &TransferDCOrder{
		Nonce:        nonce,
		TargetBearer: ownerProof,
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, test.RandomBytes(3), swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, []byte{0}, script.PredicateAlwaysTrue())
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
		dcTransfers[i] = newTransferDC(t, 100, []byte{6}, bytes32[:], swapId, script.PredicateAlwaysTrue())
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer, bytes32[:])
	}
	swapTx := newSwapOrderWithDCTransfers(script.PredicateAlwaysTrue(), 200, dcTransfers, transferIds, proofs)
	swapTxProto := newPBTransactionOrder(swapId, script.PredicateArgumentEmpty(), 2, swapTx)
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
		dcTransfers[i] = newTransferDC(t, 100, []byte{6}, bytes32[:], swapId, script.PredicateAlwaysTrue())
		proofs[i] = testblock.CreateProof(t, dcTransfers[i], signer, bytes32[:])
	}
	swapTx := newSwapOrderWithDCTransfers(script.PredicateAlwaysTrue(), 200, dcTransfers, transferIds, proofs)
	swapTxProto := newPBTransactionOrder(swapId, script.PredicateArgumentEmpty(), 2, swapTx)
	tx, err := NewMoneyTx(systemIdentifier, swapTxProto)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newSwapOrderWithWrongOwnerCondition(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysFalse())
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder(swapId, script.PredicateArgumentEmpty(), 2, newSwapOrder(dcTransfer, transferId, proofs))
	tx, err := NewMoneyTx(systemIdentifier, order)
	require.NoError(t, err)
	require.IsType(t, tx, &swapWrapper{})
	return tx.(*swapWrapper)
}

func newInvalidTargetBearerSwap(t *testing.T, signer abcrypto.Signer) *swapWrapper {
	id := uint256.NewInt(1)
	id32 := id.Bytes32()
	transferId := id32[:]
	swapId := calculateSwapID(id)
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
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
	dcTransfer := newTransferDC(t, 100, []byte{6}, transferId, swapId, script.PredicateAlwaysTrue())
	proofs := []*block.BlockProof{testblock.CreateProof(t, dcTransfer, signer, id32[:])}
	order := newPBTransactionOrder(swapId, script.PredicateArgumentEmpty(), 2, newSwapOrder(dcTransfer, transferId, proofs))
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
