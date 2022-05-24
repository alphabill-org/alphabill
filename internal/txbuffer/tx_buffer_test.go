package txbuffer

import (
	"context"
	gocrypto "crypto"
	"sync"
	"testing"
	"time"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
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
	defer buffer.Close()
	require.Equal(t, testBufferSize, buffer.maxSize)
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestAddTx_TxIsNil(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	err = buffer.Add(nil)
	require.ErrorIs(t, err, ErrTxIsNil)
}

func TestAddTx_TxIsAlreadyInTxBuffer(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	tx := testtransaction.RandomGenericBillTransfer(t)
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
	defer buffer.Close()

	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
		require.NoError(t, err)
	}

	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))

	require.ErrorIs(t, err, ErrTxBufferFull)
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestAddTx_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
	require.NoError(t, err)
	require.Equal(t, one, buffer.Count())
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestCount_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
		require.NoError(t, err)
	}
	require.Equal(t, testBufferSize, buffer.Count())
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestRemove_NotFound(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	tx := testtransaction.RandomGenericBillTransfer(t)
	err = buffer.Add(tx)
	require.NoError(t, err)
	buffer.remove("1")
	require.Equal(t, uint32(1), buffer.Count())
}

func TestRemove_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	tx := testtransaction.RandomGenericBillTransfer(t)
	err = buffer.Add(tx)
	require.NoError(t, err)

	hash := tx.Hash(gocrypto.SHA256)
	buffer.remove(string(hash))
	require.Equal(t, zero, buffer.Count())
	require.Equal(t, zero, uint32(len(buffer.transactions)))
}

func TestProcess_ProcessAllTransactions(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
	require.NoError(t, err)
	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
	require.NoError(t, err)
	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
	require.NoError(t, err)
	var c int
	go buffer.Process(context.Background(), nil, func(tx txsystem.GenericTransaction) bool {
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
	defer buffer.Close()
	err = buffer.Add(testtransaction.RandomGenericBillTransfer(t))
	require.NoError(t, err)
	context, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, func() {
		cancel()
	})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	buffer.Process(context, wg, func(tx txsystem.GenericTransaction) bool {
		return false
	})
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, uint32(1), buffer.Count())
}
