package wallet

import (
	"sync"
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestWalletShutdownTerminatesSync(t *testing.T) {
	w := New().SetBlockProcessor(DummyBlockProcessor{}).SetABClient(&DummyAlphabillClient{}).Build()

	// when Sync is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := w.Sync(0)
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
