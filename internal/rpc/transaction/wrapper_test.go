package transaction

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWrapper_InterfaceAssertion(t *testing.T) {
	var (
		pbBillTransfer        = newPBBillTransfer(test.RandomBytes(10), test.RandomBytes(32), 100)
		pbTransferTransaction = newPBTransactionOrder(test.RandomBytes(32), []byte{1}, 500, pbBillTransfer)
	)
	wr, err := New(pbTransferTransaction)
	require.NoError(t, err)

	hashValue1 := wr.Hash(crypto.SHA256)

	// The assertion works only with interfaces.
	wrapperInterface := interface{}(wr)
	transfer, ok := wrapperInterface.(state.Transfer)
	require.True(t, ok)

	require.Equal(t, state.TypeTransfer, transfer.Type())

	// Bill transfer fields
	assert.Equal(t, pbTransferTransaction.Timeout, transfer.Timeout())
	assert.Equal(t, pbBillTransfer.NewBearer, transfer.NewBearer())
	assert.Equal(t, pbBillTransfer.Backlink, transfer.Backlink())
	assert.Equal(t, pbBillTransfer.TargetValue, transfer.TargetValue())

	// Type switch can be used to find which interface is satisfied
	// If a transfer with exactly same fields would be added, then the switch will find the first one.
	// Not a problem at the moment. A solution is to use the Type() function.
	switch w := wrapperInterface.(type) {
	case state.Transfer:
		assert.Equal(t, pbTransferTransaction.Timeout, w.Timeout())
		assert.Equal(t, pbBillTransfer.NewBearer, w.NewBearer())
		assert.Equal(t, pbBillTransfer.Backlink, w.Backlink())
		assert.Equal(t, pbBillTransfer.TargetValue, w.TargetValue())
		hashValue2 := w.Hash(crypto.SHA256)
		assert.Equal(t, hashValue1, hashValue2)
	case state.DustTransfer:
		require.Fail(t, "Should not be dust transfer")
	default:
		require.Fail(t, "Should find the correct type")
	}
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

func newPBBillTransfer(newBearer, backlink []byte, targetValue uint64) *BillTransfer {
	return &BillTransfer{
		NewBearer:   newBearer,
		Backlink:    backlink,
		TargetValue: targetValue,
	}
}
