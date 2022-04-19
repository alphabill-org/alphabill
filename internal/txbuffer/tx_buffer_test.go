package txbuffer

import (
	"context"
	gocrypto "crypto"
	"sync"
	"testing"
	"time"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"github.com/stretchr/testify/require"
)

const (
	zero           uint32 = 0
	one            uint32 = 1
	testBufferSize uint32 = 10
)

func TestNewTxBuffer_InvalidNegative(t *testing.T) {
	_, err := New(zero, gocrypto.SHA256)
	require.ErrorIs(t, err, ErrInvalidMaxSize)
}
func TestNewTxBuffer_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, buffer)
	require.Equal(t, testBufferSize, buffer.maxSize)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestAddTx_TxIsNil(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	err = buffer.Add(nil)
	require.ErrorIs(t, err, ErrTxIsNil)
}

func TestAddTx_TxIsAlreadyInTxBuffer(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	tx := newRandomTx(t)
	err = buffer.Add(tx)
	require.NoError(t, err)
	err = buffer.Add(tx)

	require.ErrorIs(t, err, ErrTxInBuffer)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestAddTx_TxBufferFull(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)

	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(newRandomTx(t))
		require.NoError(t, err)
	}

	err = buffer.Add(newRandomTx(t))

	require.ErrorIs(t, err, ErrTxBufferFull)
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestAddTx_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	err = buffer.Add(newRandomTx(t))
	require.NoError(t, err)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestCount_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(newRandomTx(t))
		require.NoError(t, err)
	}
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestGetAll_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(newRandomTx(t))
		require.NoError(t, err)
	}

	txs := buffer.GetAll()
	require.Equal(t, testBufferSize, uint32(cap(txs)))
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestRemove_NotFound(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)

	err = buffer.Remove("1")
	require.ErrorIs(t, err, ErrTxNotFound)
}

func TestRemove_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)

	tx := newRandomTx(t)
	err = buffer.Add(tx)
	require.NoError(t, err)

	hash, err := tx.Hash(gocrypto.SHA256)
	require.NoError(t, err)
	err = buffer.Remove(string(hash))
	require.NoError(t, err)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestProcess_ProcessAllTransactions(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	err = buffer.Add(newRandomTx(t))
	require.NoError(t, err)
	err = buffer.Add(newRandomTx(t))
	require.NoError(t, err)
	err = buffer.Add(newRandomTx(t))
	require.NoError(t, err)
	var c int
	go buffer.Process(context.Background(), nil, func(tx *transaction.Transaction) bool {
		c++
		return true
	})
	require.Eventually(t, func() bool {
		return c == 3
	}, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool {
		return uint32(0) == buffer.Count()
	}, test.WaitDuration, test.WaitTick)
}

func TestProcess_CancelProcess(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	err = buffer.Add(newRandomTx(t))
	require.NoError(t, err)
	context, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, func() {
		cancel()
	})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	buffer.Process(context, wg, func(tx *transaction.Transaction) bool {
		return false
	})
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, uint32(1), buffer.Count())
}

func newRandomTx(t *testing.T) *transaction.Transaction {
	t.Helper()
	return testtransaction.RandomBillTransfer()
}
