package money

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWrapper_InterfaceAssertion(t *testing.T) {
	var (
		pbBillTransfer = newPBBillTransfer(test.RandomBytes(10), 100, test.RandomBytes(32))
		pbTransaction  = newPBTransactionOrder(test.RandomBytes(32), []byte{1}, 500, pbBillTransfer)
	)
	genericTx, err := NewMoneyTx(systemIdentifier, pbTransaction)
	require.NoError(t, err)

	hashValue1 := genericTx.Hash(crypto.SHA256)

	// Type switch can be used to find which interface is satisfied
	// If a transfer with exactly same fields would be added, then the switch will find the first one.
	// Not a problem at the moment.
	switch w := genericTx.(type) {
	case Transfer:
		assert.Equal(t, pbTransaction.Timeout, w.Timeout())
		assert.Equal(t, pbBillTransfer.NewBearer, w.NewBearer())
		assert.Equal(t, pbBillTransfer.Backlink, w.Backlink())
		assert.Equal(t, pbBillTransfer.TargetValue, w.TargetValue())
		hashValue2 := w.Hash(crypto.SHA256)
		assert.Equal(t, hashValue1, hashValue2)
	case TransferDC:
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
	genericTx, err := NewMoneyTx(systemIdentifier, pbTransaction)
	require.NoError(t, err)
	transfer, ok := genericTx.(Transfer)
	require.True(t, ok)

	assert.Equal(t, toUint256(pbTransaction.UnitId), transfer.UnitID())
	assert.Equal(t, pbTransaction.OwnerProof, transfer.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, transfer.Timeout())
	assert.NotNil(t, genericTx.Hash(crypto.SHA256))
	assert.Equal(t, pbTransaction.SystemId, transfer.SystemID())

	assert.Equal(t, pbBillTransfer.NewBearer, transfer.NewBearer())
	assert.Equal(t, pbBillTransfer.Backlink, transfer.Backlink())
	assert.Equal(t, pbBillTransfer.TargetValue, transfer.TargetValue())

	assert.NotEmpty(t, transfer.SigBytes())
}

func TestWrapper_TransferDC(t *testing.T) {
	var (
		pbTransferDC  = newPBTransferDC(test.RandomBytes(32), test.RandomBytes(32), 777, test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbTransferDC)
	)
	genericTx, err := NewMoneyTx(systemIdentifier, pbTransaction)
	require.NoError(t, err)
	transfer, ok := genericTx.(TransferDC)
	require.True(t, ok)
	assert.NotNil(t, genericTx.Hash(crypto.SHA256))

	requireTransferDCEquals(t, pbTransferDC, pbTransaction, transfer)

	assert.NotEmpty(t, transfer.SigBytes())
}

func TestWrapper_Split(t *testing.T) {
	var (
		pbSplit       = newPBBillSplit(777, test.RandomBytes(32), 888, test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbSplit)
	)
	genericTx, err := NewMoneyTx(systemIdentifier, pbTransaction)
	require.NoError(t, err)
	split, ok := genericTx.(Split)
	require.True(t, ok)

	assert.NotNil(t, genericTx.Hash(crypto.SHA256))
	assert.Equal(t, pbTransaction.SystemId, split.SystemID())

	assert.Equal(t, toUint256(pbTransaction.UnitId), split.UnitID())
	assert.Equal(t, pbTransaction.OwnerProof, split.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, split.Timeout())

	assert.Equal(t, pbSplit.Amount, split.Amount())
	assert.Equal(t, pbSplit.TargetBearer, split.TargetBearer())
	assert.Equal(t, pbSplit.RemainingValue, split.RemainingValue())
	assert.Equal(t, pbSplit.Backlink, split.Backlink())

	assert.NotEmpty(t, split.SigBytes())

	// sameShardId input calculation
	actualPrndSh := split.HashForIdCalculation(crypto.SHA256)

	hasher := crypto.SHA256.New()
	idBytes := split.UnitID().Bytes32()
	hasher.Write(idBytes[:])
	hasher.Write(util.Uint64ToBytes(split.Amount()))
	hasher.Write(split.TargetBearer())
	hasher.Write(util.Uint64ToBytes(split.RemainingValue()))
	hasher.Write(split.Backlink())
	hasher.Write(util.Uint64ToBytes(split.Timeout()))
	expectedPrndSh := hasher.Sum(nil)

	require.Equal(t, expectedPrndSh, actualPrndSh)

	units := genericTx.TargetUnits(crypto.SHA256)
	require.Len(t, units, 2)
	require.Equal(t, genericTx.UnitID(), units[0])
	require.Equal(t, utiltx.SameShardId(genericTx.UnitID(), expectedPrndSh), units[1])
}

func TestWrapper_Swap(t *testing.T) {
	var (
		pbTransferDC            = newPBTransferDC(test.RandomBytes(32), test.RandomBytes(32), 777, test.RandomBytes(32))
		pbTransferDCTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 444, pbTransferDC)
		pbBlockProof            = &block.BlockProof{}
		pbSwap                  = newPBSwap(
			test.RandomBytes(32),
			[][]byte{test.RandomBytes(10)},
			[]*txsystem.Transaction{pbTransferDCTransaction},
			[]*block.BlockProof{pbBlockProof},
			777)
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbSwap)
	)
	genericTx, err := NewMoneyTx(systemIdentifier, pbTransaction)
	require.NoError(t, err)
	swap, ok := genericTx.(Swap)
	require.True(t, ok)

	assert.NotNil(t, genericTx.Hash(crypto.SHA256))
	assert.Equal(t, pbTransaction.SystemId, swap.SystemID())
	assert.Equal(t, toUint256(pbTransaction.UnitId), swap.UnitID())
	assert.Equal(t, pbTransaction.OwnerProof, swap.OwnerProof())
	assert.Equal(t, pbTransaction.Timeout, swap.Timeout())

	assert.Equal(t, pbSwap.OwnerCondition, swap.OwnerCondition())
	assert.Equal(t, []*uint256.Int{uint256.NewInt(0).SetBytes(pbSwap.BillIdentifiers[0])}, swap.BillIdentifiers())
	assert.NotEmpty(t, swap.DCTransfers())
	for _, sdt := range swap.DCTransfers() {
		requireTransferDCEquals(t, pbTransferDC, pbTransferDCTransaction, sdt)
	}
	require.Len(t, swap.Proofs(), 1)
	for _, proof := range swap.Proofs() {
		require.Equal(t, pbBlockProof, proof)
	}
	assert.Equal(t, pbSwap.TargetValue, swap.TargetValue())

	require.NotEmpty(t, swap.SigBytes())
}

func TestWrapper_DifferentPartitionTx(t *testing.T) {
	_, err := NewMoneyTx(systemIdentifier, createNonMoneyTx())
	require.ErrorContains(t, err, "transaction has invalid system identifier")
}

func TestUint256Hashing(t *testing.T) {
	// Verifies that the uint256 bytes are equals to the byte array it was made from.
	// So it doesn't matter if hash is calculated from the byte array from the uint256 byte array.
	b32 := test.RandomBytes(32)
	b32Int := uint256.NewInt(0).SetBytes(b32)
	bytes32 := b32Int.Bytes32()
	assert.Equal(t, b32, bytes32[:])

	b33 := test.RandomBytes(33)
	b33Int := uint256.NewInt(0).SetBytes(b33)
	bytes32 = b33Int.Bytes32()
	assert.Equal(t, b33[1:], bytes32[:])

	b1 := test.RandomBytes(1)
	b1Int := uint256.NewInt(0).SetBytes(b1)
	expected := [32]byte{}
	expected[31] = b1[0]
	assert.Equal(t, expected, b1Int.Bytes32())
}

func TestUint256Hashing_LeadingZeroByte(t *testing.T) {
	b32 := test.RandomBytes(32)
	b32[0] = 0x00
	b32Int := uint256.NewInt(0).SetBytes(b32)
	b32IntBytes := b32Int.Bytes32() // Bytes32() works, Bytes() does not

	hasher := crypto.SHA256.New()
	hasher.Write(b32IntBytes[:])
	h1 := hasher.Sum(nil)

	hasher.Reset()
	hasher.Write(b32)
	h2 := hasher.Sum(nil)

	require.EqualValues(t, h2, h1)
}

// requireTransferDCEquals compares protobuf object fields and the state.TransferDC corresponding getters to be equal.
func requireTransferDCEquals(t *testing.T, pbTransferDC *TransferDCOrder, pbTransaction *txsystem.Transaction, transfer TransferDC) {
	assert.Equal(t, pbTransaction.SystemId, transfer.SystemID())
	require.Equal(t, toUint256(pbTransaction.UnitId), transfer.UnitID())
	require.Equal(t, pbTransaction.OwnerProof, transfer.OwnerProof())
	require.Equal(t, pbTransaction.Timeout, transfer.Timeout())

	require.Equal(t, pbTransferDC.TargetBearer, transfer.TargetBearer())
	require.Equal(t, pbTransferDC.Backlink, transfer.Backlink())
	require.Equal(t, pbTransferDC.Nonce, transfer.Nonce())
	require.Equal(t, pbTransferDC.TargetValue, transfer.TargetValue())

	assert.NotEmpty(t, transfer.SigBytes())
}

func newPBTransactionOrder(id, ownerProof []byte, timeout uint64, attr proto.Message) *txsystem.Transaction {
	to := &txsystem.Transaction{
		SystemId:              systemIdentifier,
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

func newPBBillTransfer(newBearer []byte, targetValue uint64, backlink []byte) *TransferOrder {
	return &TransferOrder{
		NewBearer:   newBearer,
		TargetValue: targetValue,
		Backlink:    backlink,
	}
}

func newPBTransferDC(nonce, targetBearer []byte, targetValue uint64, backlink []byte) *TransferDCOrder {
	return &TransferDCOrder{
		Nonce:        nonce,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
	}
}

func newPBBillSplit(amount uint64, targetBearer []byte, remainingValue uint64, backlink []byte) *SplitOrder {
	return &SplitOrder{
		Amount:         amount,
		TargetBearer:   targetBearer,
		RemainingValue: remainingValue,
		Backlink:       backlink,
	}
}

func newPBSwap(ownerCondition []byte, billIdentifiers [][]byte, dcTransfers []*txsystem.Transaction, proofs []*block.BlockProof, targetValue uint64) *SwapOrder {
	return &SwapOrder{
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
