package wallet

import (
	"context"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/stretchr/testify/require"
)

func TestWalletShutdownTerminatesSync(t *testing.T) {
	w := New().SetBlockProcessor(dummyBlockProcessor{}).SetABClient(&clientmock.MockAlphabillClient{}).Build()

	ctx, cancel := context.WithCancel(context.Background())

	// when Sync is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := w.Sync(ctx, 0)
		require.NoError(t, err)
		wg.Done()
	}()

	// and wallet is closed
	cancel()

	// then Sync goroutine should end
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
}

type dummyBlockProcessor struct {
}

func (p dummyBlockProcessor) ProcessBlock(*block.Block) error {
	return nil
}
