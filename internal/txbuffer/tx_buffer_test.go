package txbuffer

import (
	"context"
	gocrypto "crypto"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

const (
	zero           uint32 = 0
	one            uint32 = 1
	testBufferSize uint32 = 10
)

func TestNewTxBuffer_InvalidMaxSize(t *testing.T) {
	_, err := New(zero, gocrypto.SHA256)
	require.ErrorIs(t, err, ErrInvalidMaxSize)
}
func TestNewTxBuffer_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NotNil(t, buffer)
	defer buffer.Close()
	require.NoError(t, err)
	require.EqualValues(t, zero, len(buffer.transactionsCh))
	require.EqualValues(t, zero, len(buffer.transactions))
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

	tx := testtransaction.NewTransactionOrder(t)
	err = buffer.Add(tx)
	require.NoError(t, err)

	err = buffer.Add(tx)
	require.ErrorIs(t, err, ErrTxInBuffer)
	require.EqualValues(t, one, len(buffer.transactionsCh))
	require.EqualValues(t, one, len(buffer.transactions))
}

func TestAddTx_TxBufferFull(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()

	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(testtransaction.NewTransactionOrder(t))
		require.NoError(t, err)
	}

	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.ErrorIs(t, err, ErrTxBufferFull)
	require.EqualValues(t, testBufferSize, len(buffer.transactionsCh))
	require.Equal(t, testBufferSize, uint32(len(buffer.transactions)))
}

func TestAddTx_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)
	require.EqualValues(t, one, len(buffer.transactionsCh))
	require.Equal(t, one, uint32(len(buffer.transactions)))
}

func TestCount_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	for i := uint32(0); i < testBufferSize; i++ {
		err = buffer.Add(testtransaction.NewTransactionOrder(t))
		require.NoError(t, err)
	}
	require.EqualValues(t, testBufferSize, len(buffer.transactionsCh))
	require.EqualValues(t, testBufferSize, len(buffer.transactions))
}

func TestRemove_NotFound(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()
	tx := testtransaction.NewTransactionOrder(t)
	err = buffer.Add(tx)
	require.NoError(t, err)
	buffer.removeFromIndex("1")
	require.EqualValues(t, 1, len(buffer.transactionsCh))
}

func TestRemove_Ok(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()

	tx := testtransaction.NewTransactionOrder(t)
	err = buffer.Add(tx)
	require.NoError(t, err)

	hash := tx.Hash(gocrypto.SHA256)
	buffer.removeFromIndex(string(hash))
	// the tx is removed from the index map but is still in chan!
	require.EqualValues(t, 1, len(buffer.transactionsCh))
	require.EqualValues(t, 0, len(buffer.transactions))
}

func TestProcess_ProcessAllTransactions(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()

	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)
	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)
	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)

	var c uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer.Process(context.Background(), func(tx *types.TransactionOrder) bool {
			atomic.AddUint32(&c, 1)
			return true
		})
	}()

	require.Eventually(t, func() bool {
		return atomic.LoadUint32(&c) == 3
	}, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool {
		return len(buffer.transactionsCh) == 0
	}, test.WaitDuration, test.WaitTick)
}

func TestProcess_CloseQuitsProcess(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)

	var c uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer.Process(context.Background(), func(tx *types.TransactionOrder) bool {
			atomic.AddUint32(&c, 1)
			return true
		})
	}()

	buffer.Close()
	select {
	case <-time.After(test.WaitDuration):
		t.Error("buffer.Process didn't quit within timeout")
	case <-done:
		require.EqualValues(t, 0, atomic.LoadUint32(&c), "unexpectedly process callback has been called")
	}
}

func TestProcess_CancelProcess(t *testing.T) {
	buffer, err := New(testBufferSize, gocrypto.SHA256)
	require.NoError(t, err)
	defer buffer.Close()

	err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer.Process(ctx, func(tx *types.TransactionOrder) bool {
			cancel()
			<-ctx.Done()
			return false
		})
	}()
	// processing the tx should trigger cancellation of the process loop
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
	require.EqualValues(t, 0, len(buffer.transactionsCh))
}
