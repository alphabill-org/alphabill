package txbuffer

import (
	"testing"

	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"

	"github.com/stretchr/testify/require"
)

const (
	zero           uint32 = 0
	one            uint32 = 1
	testBufferSize uint32 = 10
)

func TestNewTxBuffer_InvalidNegative(t *testing.T) {
	_, err := New(zero)
	require.ErrorIs(t, err, ErrInvalidMaxSize)
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
	require.ErrorIs(t, err, ErrTxIsNil)
}

func TestAddTx_TxIsAlreadyInTxBuffer(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	tx := NewRandomTx(t)
	err = buffer.Add(tx)
	require.NoError(t, err)
	err = buffer.Add(tx)

	require.ErrorIs(t, err, ErrTxInBuffer)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestAddTx_TxBufferFull(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(NewRandomTx(t))
		require.NoError(t, err)
	}

	err = buffer.Add(NewRandomTx(t))

	require.ErrorIs(t, err, ErrTxBufferFull)
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestAddTx_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	err = buffer.Add(NewRandomTx(t))
	require.NoError(t, err)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestCount_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(NewRandomTx(t))
		require.NoError(t, err)
	}
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestGetAll_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(NewRandomTx(t))
		require.NoError(t, err)
	}

	txs := buffer.GetAll()
	require.Equal(t, testBufferSize, uint32(cap(txs)))
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestRemove_NotFound(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	err = buffer.Remove("1")
	require.ErrorIs(t, err, ErrTxNotFound)
}

func TestRemove_Ok(t *testing.T) {
	buffer, err := New(testBufferSize)
	require.NoError(t, err)

	tx := NewRandomTx(t)
	err = buffer.Add(tx)
	require.NoError(t, err)

	err = buffer.Remove(tx.IDHash())
	require.NoError(t, err)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func NewRandomTx(t *testing.T) transaction.GenericTransaction {
	t.Helper()
	tx, err := transaction.New(testtransaction.RandomBillTransfer())
	require.NoError(t, err)
	return tx
}
