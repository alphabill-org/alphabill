package wallet

import (
	"context"
	"errors"
	"sync"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/stretchr/testify/require"
)

func TestWalletSendFunction_RetryTxWhenTxBufferIsFull(t *testing.T) {
	// setup wallet
	mockClient := clientmock.NewMockAlphabillClient(clientmock.WithMaxBlockNumber(0), clientmock.WithBlocks(map[uint64]*types.Block{}))
	w := &Wallet{
		AlphabillClient: mockClient,
		BlockProcessor:  &dummyBlockProcessor{},
	}

	// mock TxBufferFullErrMessage
	mockClient.SetTxResponse(errors.New(txBufferFullErrMsg))

	// send tx
	err := w.SendTransaction(context.Background(), &types.TransactionOrder{}, &SendOpts{RetryOnFullTxBuffer: true})

	// verify send tx error
	require.ErrorIs(t, err, ErrFailedToBroadcastTx)

	// verify txs were broadcast multiple times
	require.Eventually(t, func() bool {
		return len(mockClient.GetRecordedTransactions()) == maxTxFailedTries
	}, test.WaitDuration, test.WaitTick)
}

func TestWalletSendFunction_RetryCanBeCanceledByUser(t *testing.T) {
	// setup wallet
	mockClient := clientmock.NewMockAlphabillClient(clientmock.WithMaxBlockNumber(0), clientmock.WithBlocks(map[uint64]*types.Block{}))
	w := &Wallet{
		AlphabillClient: mockClient,
		BlockProcessor:  &dummyBlockProcessor{},
	}

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(errors.New(txBufferFullErrMsg))

	// send tx
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	var sendError error
	go func() {
		sendError = w.SendTransaction(ctx, &types.TransactionOrder{}, &SendOpts{RetryOnFullTxBuffer: true})
		wg.Done()
	}()

	// when context is canceled
	cancel()

	// then sendError returns immediately
	wg.Wait()
	require.ErrorIs(t, sendError, ErrTxRetryCanceled)

	// and only the initial transaction should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
}
