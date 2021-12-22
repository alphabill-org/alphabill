package transaction

import (
	"crypto"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWrapper_InterfaceAssertion(t *testing.T) {
	var (
		pbBillTransfer = newPBBillTransfer(test.RandomBytes(10), 100, test.RandomBytes(32))
		pbTransaction  = newPBTransactionOrder(test.RandomBytes(32), []byte{1}, 500, pbBillTransfer)
	)
	genericTx, err := New(pbTransaction)
	require.NoError(t, err)

	hashValue1 := genericTx.Hash(crypto.SHA256)

	// Type switch can be used to find which interface is satisfied
	// If a transfer with exactly same fields would be added, then the switch will find the first one.
	// Not a problem at the moment.
	switch w := genericTx.(type) {
	case state.Transfer:
		assert.Equal(t, pbTransaction.Timeout, w.Timeout())
		assert.Equal(t, pbBillTransfer.NewBearer, w.NewBearer())
		assert.Equal(t, pbBillTransfer.Backlink, w.Backlink())
		assert.Equal(t, pbBillTransfer.TargetValue, w.TargetValue())
		hashValue2 := w.Hash(crypto.SHA256)
		assert.Equal(t, hashValue1, hashValue2)
	case state.TransferDC:
		require.Fail(t, "Should not be transferDC")
	default:
		require.Fail(t, "Should find the correct type")
	}
}

func TestWrapper_Transfer(t *testing.T) {
	var (
		pbBillTransfer = newPBBillTransfer(test.RandomBytes(10), 100, test.RandomBytes(32))
		pbTransaction  = newPBTransactionOrder(test.RandomBytes(32), []byte{1}, 500, pbBillTransfer)
	)
	genericTx, err := New(pbTransaction)
	require.NoError(t, err)
	transfer, ok := genericTx.(state.Transfer)
	require.True(t, ok)

	assert.Equal(t, toUint256(pbTransaction.UnitId), transfer.UnitId())
	assert.Equal(t, pbTransaction.OwnerProof, transfer.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, transfer.Timeout())
	assert.NotNil(t, genericTx.Hash(crypto.SHA256))

	assert.Equal(t, pbBillTransfer.NewBearer, transfer.NewBearer())
	assert.Equal(t, pbBillTransfer.Backlink, transfer.Backlink())
	assert.Equal(t, pbBillTransfer.TargetValue, transfer.TargetValue())
}

func TestWrapper_TransferDC(t *testing.T) {
	var (
		pbTransferDC  = newPBTransferDC(test.RandomBytes(32), test.RandomBytes(32), 777, test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbTransferDC)
	)
	genericTx, err := New(pbTransaction)
	require.NoError(t, err)
	transfer, ok := genericTx.(state.TransferDC)
	require.True(t, ok)
	assert.NotNil(t, genericTx.Hash(crypto.SHA256))

	requireTransferDCEquals(t, pbTransferDC, pbTransaction, transfer)
}

func TestWrapper_Split(t *testing.T) {
	var (
		pbSplit       = newPBBillSplit(777, test.RandomBytes(32), 888, test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbSplit)
	)
	genericTx, err := New(pbTransaction)
	require.NoError(t, err)
	split, ok := genericTx.(state.Split)
	require.True(t, ok)

	assert.NotNil(t, genericTx.Hash(crypto.SHA256))

	assert.Equal(t, toUint256(pbTransaction.UnitId), split.UnitId())
	assert.Equal(t, pbTransaction.OwnerProof, split.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, split.Timeout())

	assert.Equal(t, pbSplit.Amount, split.Amount())
	assert.Equal(t, pbSplit.TargetBearer, split.TargetBearer())
	assert.Equal(t, pbSplit.RemainingValue, split.RemainingValue())
	assert.Equal(t, pbSplit.Backlink, split.Backlink())
}

func TestWrapper_Swap(t *testing.T) {
	var (
		pbTransferDC            = newPBTransferDC(test.RandomBytes(32), test.RandomBytes(32), 777, test.RandomBytes(32))
		pbTransferDCTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 444, pbTransferDC)
		pbSwap                  = newPBSwap(
			test.RandomBytes(32),
			[][]byte{test.RandomBytes(10)},
			[]*Transaction{pbTransferDCTransaction},
			[][]byte{test.RandomBytes(32)},
			777)
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbSwap)
	)
	genericTx, err := New(pbTransaction)
	require.NoError(t, err)
	swap, ok := genericTx.(state.Swap)
	require.True(t, ok)

	assert.NotNil(t, genericTx.Hash(crypto.SHA256))

	assert.Equal(t, toUint256(pbTransaction.UnitId), swap.UnitId())
	assert.Equal(t, pbTransaction.OwnerProof, swap.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, swap.Timeout())

	assert.Equal(t, pbSwap.OwnerCondition, swap.OwnerCondition())
	assert.Equal(t, []*uint256.Int{uint256.NewInt(0).SetBytes(pbSwap.BillIdentifiers[0])}, swap.BillIdentifiers())
	assert.NotEmpty(t, swap.DCTransfers())
	for _, sdt := range swap.DCTransfers() {
		requireTransferDCEquals(t, pbTransferDC, pbTransferDCTransaction, sdt)
	}
	assert.Equal(t, pbSwap.Proofs, swap.Proofs())
	assert.Equal(t, pbSwap.TargetValue, swap.TargetValue())
}

func TestUint256Hashing(t *testing.T) {
	// Verifies that the uint256 bytes are equals to the byte array it was made from.
	// So it doesn't matter if hash is calculated from the byte array from the uint256 byte array.
	b32 := test.RandomBytes(32)
	b32Int := uint256.NewInt(0).SetBytes(b32)
	assert.Equal(t, b32, b32Int.Bytes())

	b33 := test.RandomBytes(33)
	b33Int := uint256.NewInt(0).SetBytes(b33)
	assert.Equal(t, b33[1:], b33Int.Bytes())

	b1 := test.RandomBytes(1)
	b1Int := uint256.NewInt(0).SetBytes(b1)
	expected := [32]byte{}
	expected[31] = b1[0]
	assert.Equal(t, expected, b1Int.Bytes32())
}

// requireTransferDCEquals compares protobuf object fields and the state.TransferDC corresponding getters to be equal.
func requireTransferDCEquals(t *testing.T, pbTransferDC *TransferDC, pbTransaction *Transaction, transfer state.TransferDC) {
	require.Equal(t, toUint256(pbTransaction.UnitId), transfer.UnitId())
	require.Equal(t, pbTransaction.OwnerProof, transfer.OwnerProof())
	require.Equal(t, pbTransaction.Timeout, transfer.Timeout())

	require.Equal(t, pbTransferDC.TargetBearer, transfer.TargetBearer())
	require.Equal(t, pbTransferDC.Backlink, transfer.Backlink())
	require.Equal(t, pbTransferDC.Nonce, transfer.Nonce())
	require.Equal(t, pbTransferDC.TargetValue, transfer.TargetValue())
}

func newPBTransactionOrder(id, ownerProof []byte, timeout uint64, attr proto.Message) *Transaction {
	to := &Transaction{
		UnitId:                id,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	err := anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	if err != nil {
		panic(err)
	}
	return to
}

func newPBBillTransfer(newBearer []byte, targetValue uint64, backlink []byte) *BillTransfer {
	return &BillTransfer{
		NewBearer:   newBearer,
		TargetValue: targetValue,
		Backlink:    backlink,
	}
}

func newPBTransferDC(nonce, targetBearer []byte, targetValue uint64, backlink []byte) *TransferDC {
	return &TransferDC{
		Nonce:        nonce,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
	}
}

func newPBBillSplit(amount uint64, targetBearer []byte, remainingValue uint64, backlink []byte) *BillSplit {
	return &BillSplit{
		Amount:         amount,
		TargetBearer:   targetBearer,
		RemainingValue: remainingValue,
		Backlink:       backlink,
	}
}

func newPBSwap(ownerCondition []byte, billIdentifiers [][]byte, dcTransfers []*Transaction, proofs [][]byte, targetValue uint64) *Swap {
	return &Swap{
		OwnerCondition:  ownerCondition,
		BillIdentifiers: billIdentifiers,
		DcTransfers:     dcTransfers,
		Proofs:          proofs,
		TargetValue:     targetValue,
	}
}

func toUint256(b []byte) *uint256.Int {
	return uint256.NewInt(0).SetBytes(b)
}
