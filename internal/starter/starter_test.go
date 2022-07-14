package starter

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/async"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
)

func TestStartAndWait(t *testing.T) {
	test.MustRunInTime(t, time.Second, func() {
		ctx, _ := async.WithWaitGroup(context.Background())
		wg := sync.WaitGroup{}

		wg.Add(1)
		startFunc := func(ctx context.Context) {
			wg.Done()
		}

		err := StartAndWait(ctx, "test-component", startFunc)
		require.NoError(t, err)

		wg.Wait()
	})
}

func TestStartAndWait_MissingWaitGroup(t *testing.T) {
	startFunc := func(ctx context.Context) {
		t.Fail() // Shouldn't run the start function
	}
	err := StartAndWait(context.Background(), "test-component", startFunc)
	require.Error(t, err)
}

func TestStartAndWait_SignalCatching(t *testing.T) {
	test.MustRunInTime(t, 3*time.Second, func() {
		ctx, asyncWg := async.WithWaitGroup(context.Background())

		startFunc := func(ctx context.Context) {
			asyncWg.Add(1) // This will make the StartAndWait wait
			go func() {
				log.Debug("waiting for context done to be called")
				<-ctx.Done()
				log.Debug("context done called")
				// When ctx.Done is called, release the other wait group.
				asyncWg.Done()
			}()
		}

		go func() {
			// Waiting for the start-and-wait to start listening for signals
			time.Sleep(50 * time.Millisecond)
			log.Debug("Sending SIGTERM to PID: %d", syscall.Getpid())
			err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
			require.NoError(t, err)
		}()

		// This will block until it's closed or nothing to wait by the wait group
		err := StartAndWait(ctx, "test-component", startFunc)
		require.NoError(t, err)
	})
}
