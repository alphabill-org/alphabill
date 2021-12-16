package txbuffer

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
	"testing"
)

const (
	zero           uint32 = 0
	one            uint32 = 1
	testBufferSize uint32 = 10
)

func TestNewTxBuffer_InvalidNegative(t *testing.T) {
	_, err := New(zero)
	require.Error(t, err)
	require.Equal(t, err, ErrInvalidMaxSize)
}
func TestNewTxBuffer_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	require.NotNil(t, buffer)
	require.Equal(t, testBufferSize, buffer.maxSize)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestAddTx_TxIsNil(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	err = buffer.Add(nil)
	require.Error(t, err)
	require.Equal(t, ErrTxIsNil, err)
}

func TestAddTx_TxIsAlreadyInTxBuffer(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	tx := test.RandomPaymentOrder(domain.PaymentTypeTransfer)
	err = buffer.Add(tx)
	require.NoError(t, err)
	err = buffer.Add(tx)

	require.Error(t, err)
	require.Equal(t, ErrTxInBuffer, err)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestAddTx_TxBufferFull(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
		require.NoError(t, err)
	}

	err = buffer.Add(test.RandomPaymentOrder(domain.PaymentTypeTransfer))

	require.Error(t, err)
	require.Equal(t, ErrTxBufferFull, err)
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestAddTx_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	err = buffer.Add(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
	require.NoError(t, err)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestCount_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
		require.NoError(t, err)
	}
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestGetAll_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
		require.NoError(t, err)
	}

	txs := buffer.GetAll()
	require.Equal(t, testBufferSize, uint32(len(txs)))
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestRemove_NotFound(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	err = buffer.Remove("1")
	require.Error(t, err)
	require.Equal(t, ErrTxNotFound, err)
}

func TestRemove_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	tx := test.RandomPaymentOrder(domain.PaymentTypeTransfer)
	err = buffer.Add(tx)
	require.NoError(t, err)

	err = buffer.Remove(tx.ID())
	require.NoError(t, err)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}
