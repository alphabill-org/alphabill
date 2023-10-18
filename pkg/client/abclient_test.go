package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

// TestRaceConditions meant to be run with -race flag
func TestRaceConditions(t *testing.T) {
	// start mock ab server
	serviceServer := testserver.NewTestAlphabillServiceServer()
	server, addr := testserver.StartServer(serviceServer)
	t.Cleanup(server.GracefulStop)

	// create ab client
	abclient, err := New(AlphabillClientConfig{Uri: addr.String()}, logger.New(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = abclient.Close() })

	// do async operations on abclient
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		_ = abclient.SendTransaction(context.Background(), createRandomTx())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetBlock(context.Background(), 1)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetRoundNumber(context.Background())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_ = abclient.Close()
		wg.Done()
	}()

	wg.Wait()
}

func TestTimeout(t *testing.T) {
	server, client := startServerAndCreateClient(t)

	// set client timeout to a low value, so the request times out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// make GetBlock request wait for some time
	server.SetBlockFunc(0, func() *types.Block {
		time.Sleep(100 * time.Millisecond)
		return &types.Block{
			Header: &types.Header{
				PreviousBlockHash: hash.Sum256([]byte{}),
			},
			Transactions:       []*types.TransactionRecord{},
			UnicityCertificate: &types.UnicityCertificate{},
		}
	})

	// fetch the block and verify request is timed out
	b, err := client.GetBlock(ctx, 0)
	require.Nil(t, b)
	require.ErrorContains(t, err, "deadline exceeded")
}

func TestSendTransactionWithRetry_ServerError(t *testing.T) {
	server, client := startServerAndCreateClient(t)
	server.SetProcessTxError(errors.New("some error"))

	// test abclient returns error
	err := client.SendTransactionWithRetry(context.Background(), createRandomTx(), 1)
	require.ErrorContains(t, err, "failed to send transaction:")
	require.ErrorContains(t, err, "some error")
}

func TestSendTransactionWithRetry_RetryTxWhenTxBufferIsFull(t *testing.T) {
	server, client := startServerAndCreateClient(t)
	// make server return TxBufferFullErrMessage
	server.SetProcessTxError(errors.New(ErrTxBufferFull))

	// send tx
	maxTries := 2
	sendError := client.SendTransactionWithRetry(context.Background(), createRandomTx(), maxTries)

	// verify send tx error
	require.EqualError(t, sendError, ErrFailedToBroadcastTx)

	// verify txs were broadcast multiple times
	require.Eventually(t, func() bool {
		return len(server.GetProcessedTransactions()) == maxTries
	}, test.WaitDuration, test.WaitTick)
}

func TestSendTransactionWithRetry_RetryCanBeCanceledByUser(t *testing.T) {
	server, client := startServerAndCreateClient(t)

	// make server return TxBufferFullErrMessage
	server.SetProcessTxError(errors.New(ErrTxBufferFull))

	// send tx
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	var sendError error
	go func() {
		sendError = client.SendTransactionWithRetry(ctx, createRandomTx(), 3)
		wg.Done()
	}()

	// when context is canceled
	cancel()

	// then sendError returns immediately
	wg.Wait()

	// context most likely canceled while while gRPC is doing the send,
	// responds with rpc error desc "context canceled"
	require.ErrorContains(t, sendError, "canceled")

	// and most likely no transactions reached the server
	require.Len(t, server.GetProcessedTransactions(), 0)
}

func createRandomTx() *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			UnitID:         hash.Sum256([]byte{0x00}),
			ClientMetadata: &types.ClientMetadata{Timeout: 1000},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}
}

func startServerAndCreateClient(t *testing.T) (*testserver.TestAlphabillServiceServer, *AlphabillClient) {
	// start mock ab server
	serviceServer := testserver.NewTestAlphabillServiceServer()
	server, addr := testserver.StartServer(serviceServer)
	t.Cleanup(server.GracefulStop)

	// create ab client
	abclient, err := New(AlphabillClientConfig{Uri: addr.String()}, logger.New(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = abclient.Close() })
	return serviceServer, abclient
}
