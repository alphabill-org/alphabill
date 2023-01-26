package wallet

import (
	"context"
	"sync"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/stretchr/testify/require"
)

func TestWalletShutdownTerminatesSync(t *testing.T) {
	w := New().SetBlockProcessor(DummyBlockProcessor{}).SetABClient(&clientmock.MockAlphabillClient{}).Build()

	// when Sync is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := w.Sync(context.Background(), 0)
		require.NoError(t, err)
		wg.Done()
	}()

	// wait for sync to start
	go func() {
		require.Eventually(t, w.syncFlag.isSynchronizing, test.WaitDuration, test.WaitTick)
	}()

	// and wallet is closed
	w.Shutdown()

	// then Sync goroutine should end
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
}

func TestWallet_SyncToMaxBlockNumber(t *testing.T) {
	client := &clientmock.MockAlphabillClient{}
	client.SetIncrementOnFetch(true)
	w := New().SetBlockProcessor(DummyBlockProcessor{}).SetABClient(client).Build()
	client.SetMaxBlockNumber(5)
	client.SetMaxRoundNumber(10)
	err := w.SyncToMaxBlockNumber(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, uint64(5), client.GetLastRequestedBlockNumber())
}
