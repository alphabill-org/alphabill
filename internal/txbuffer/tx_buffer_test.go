package txbuffer

import (
	"context"
	"crypto"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	testBufferSize = 10
)

func Test_TxBuffer_New(t *testing.T) {
	t.Run("invalid buffer size", func(t *testing.T) {
		buffer, err := New(0, crypto.SHA256, logger.New(t))
		require.ErrorIs(t, err, ErrInvalidMaxSize)
		require.Nil(t, buffer)
	})

	t.Run("success", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)
		require.NotNil(t, buffer)
		require.Equal(t, crypto.SHA256, buffer.hashAlgorithm)
		require.NotNil(t, buffer.transactionsCh)
		require.EqualValues(t, testBufferSize, cap(buffer.transactionsCh))
		require.NotNil(t, buffer.transactions)
		require.NotNil(t, buffer.log)
	})
}

func Test_TxBuffer_Add(t *testing.T) {
	t.Run("nil tx is rejected", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)
		txh, err := buffer.Add(nil)
		require.ErrorIs(t, err, ErrTxIsNil)
		require.Nil(t, txh)
		require.Empty(t, buffer.transactions)
		require.Empty(t, buffer.transactionsCh)
	})

	t.Run("tx already in buffer", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)

		tx := testtransaction.NewTransactionOrder(t)
		txh, err := buffer.Add(tx)
		require.NoError(t, err)
		require.NotEmpty(t, txh)
		require.Len(t, buffer.transactions, 1)
		require.Len(t, buffer.transactionsCh, 1)
		require.Contains(t, buffer.transactions, string(txh))

		_, err = buffer.Add(tx)
		require.ErrorIs(t, err, ErrTxInBuffer)
		require.Len(t, buffer.transactions, 1)
		require.Len(t, buffer.transactionsCh, 1)
		require.Contains(t, buffer.transactions, string(txh))
	})

	t.Run("buffer is full", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)

		for i := 0; i < int(testBufferSize); i++ {
			_, err = buffer.Add(testtransaction.NewTransactionOrder(t))
			require.NoError(t, err)
		}

		_, err = buffer.Add(testtransaction.NewTransactionOrder(t))
		require.ErrorIs(t, err, ErrTxBufferFull)
		require.Len(t, buffer.transactions, testBufferSize)
		require.Len(t, buffer.transactionsCh, testBufferSize)
	})
}

func Test_TxBuffer_removeFromIndex(t *testing.T) {
	t.Run("tx id not in the index", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)
		tx := testtransaction.NewTransactionOrder(t)
		txh, err := buffer.Add(tx)
		require.NoError(t, err)

		buffer.removeFromIndex("1")
		require.Len(t, buffer.transactionsCh, 1)
		require.Len(t, buffer.transactions, 1)
		require.Contains(t, buffer.transactions, string(txh))
	})

	t.Run("tx id is in the index", func(t *testing.T) {
		buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
		require.NoError(t, err)

		tx := testtransaction.NewTransactionOrder(t)
		txh, err := buffer.Add(tx)
		require.NoError(t, err)
		require.NotEmpty(t, txh)

		buffer.removeFromIndex(string(txh))
		// the tx is removed from the index map but is still in chan!
		require.Len(t, buffer.transactions, 0)
		require.Len(t, buffer.transactionsCh, 1)
	})
}

func Test_TxBuffer_Process(t *testing.T) {
	buffer, err := New(testBufferSize, crypto.SHA256, logger.New(t))
	require.NoError(t, err)

	_, err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)
	_, err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)
	_, err = buffer.Add(testtransaction.NewTransactionOrder(t))
	require.NoError(t, err)

	require.Len(t, buffer.transactionsCh, 3)
	require.Len(t, buffer.transactions, 3)

	ctx, cancel := context.WithCancel(context.Background())
	var c uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer.Process(ctx, func(_ context.Context, tx *types.TransactionOrder) {
			atomic.AddUint32(&c, 1)
		})
	}()

	require.Eventually(t, func() bool { return atomic.LoadUint32(&c) == 3 }, test.WaitDuration, test.WaitTick)

	cancel()
	select {
	case <-time.After(time.Second):
		t.Fatal("buffer processor haven't shut down within timeout")
	case <-done:
		require.Empty(t, buffer.transactions)
		require.Empty(t, buffer.transactionsCh)
	}
}

func Test_TxBuffer_concurrency(t *testing.T) {
	const totalTxCnt = 20 // how many transactions to process

	buffer, err := New(10, crypto.SHA256, logger.New(t))
	require.NoError(t, err)

	// add "totalTxCnt" transactions into buffer (do not fail the test on "buffer full" error)
	go func() {
		for cnt := 0; cnt < totalTxCnt; {
			if _, err := buffer.Add(testtransaction.NewTransactionOrder(t)); err != nil {
				if !errors.Is(err, ErrTxBufferFull) {
					t.Errorf("failed to add tx: %v", err)
				}
				continue
			}
			cnt++
		}
	}()

	// consume transactions from the buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var processedCnt atomic.Int32
	go func() {
		defer close(done)
		buffer.Process(ctx, func(_ context.Context, tx *types.TransactionOrder) {
			processedCnt.Add(1)
		})
	}()

	// wait until consumer has seen the same amount of txs we generated
	require.Eventually(t, func() bool { return processedCnt.Load() == totalTxCnt }, 3*time.Second, 200*time.Millisecond)

	// shut down the tx processor
	cancel()
	select {
	case <-time.After(time.Second):
		t.Fatal("buffer processor haven't shut down within timeout")
	case <-done:
	}
}
