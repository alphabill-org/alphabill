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

type mockProcessor struct{}

func (m mockProcessor) ProcessBlock(*block.Block) error {
	return nil
}

func TestWalletShutdownTerminatesSync(t *testing.T) {
	w, err := New(
		WithBlockProcessor(DummyBlockProcessor{}),
		WithAlphabillClient(&DummyAlphabillClient{}),
		WithTxVerifier(&AlwaysValidTxVerifier{}),
	)
	require.NoError(t, err)
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

func TestNewWallet_InvalidOptions(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantErrStr string
	}{
		{
			name: "tx verifier is nil",
			opts: []Option{
				WithAlphabillClient(&clientmock.MockAlphabillClient{}),
				WithBlockProcessor(&mockProcessor{}),
			},
			wantErrStr: "tx verifier is nil",
		},
		{
			name: "block processor is nil",
			opts: []Option{
				WithAlphabillClient(&clientmock.MockAlphabillClient{}),
				WithTxVerifier(&AlwaysValidTxVerifier{}),
			},
			wantErrStr: "block processor is nil",
		},
		{
			name: "ab client is nil",
			opts: []Option{
				WithBlockProcessor(&mockProcessor{}),
				WithTxVerifier(&AlwaysValidTxVerifier{}),
			},
			wantErrStr: "alphabill client is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.opts...)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, got)
		})
	}
}
