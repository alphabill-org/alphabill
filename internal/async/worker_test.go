package async

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async/future"
	testtime "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"

	"github.com/stretchr/testify/require"
)

func TestWorker_continues(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		const (
			workerLoops = 10
		)
		var (
			ctx, ctxWaitGroup   = WithWaitGroup(context.Background())
			workerInvokeCounter int
		)

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			for {
				workerInvokeCounter++
				if workerInvokeCounter == workerLoops {
					return nil
				}
			}
		}).Start(ctx)

		ctxWaitGroup.Wait()
		require.Equal(t, workerLoops, workerInvokeCounter)
	})
}

func TestWorker_continuesWithCustomErrorChannel(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		const (
			workerLoops = 10
		)
		var (
			ctx, ctxWaitGroup   = WithWaitGroup(context.Background())
			workerInvokeCounter int
			errorFromWorkerC    = make(chan error, 1)
		)

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			for {
				workerInvokeCounter++
				if workerInvokeCounter == workerLoops {
					errorFromWorkerC <- errors.New("error from worker")
					return nil
				}
			}
		}).Start(ctx)

		ctxWaitGroup.Wait()
		require.Equal(t, workerLoops, workerInvokeCounter)
		require.Error(t, <-errorFromWorkerC)
	})
}

func TestWorker_WithFuture(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		const (
			workerLoops = 10
		)
		var (
			ctx, ctxWaitGroup   = WithWaitGroup(context.Background())
			workerInvokeCounter int
		)
		ctx, promise := WithFuture(ctx)

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			for {
				workerInvokeCounter++
				if workerInvokeCounter == workerLoops {
					return errors.New("error from worker")
				}
			}
		}).Start(ctx)

		ctxWaitGroup.Wait()
		require.Equal(t, workerLoops, workerInvokeCounter)
		value, isValid := promise.Get(ctx)
		require.True(t, isValid)
		require.Error(t, value.(error))
	})
}

func TestWorker_WithFuture_workerReturnsNil(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		ctx, promise := WithFuture(context.Background())

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			return nil
		}).Start(ctx)

		// Nil must be a valid value
		value, isValid := promise.Get(ctx)
		require.True(t, isValid)
		require.Nil(t, value)
	})
}

func TestWorker_WithFuture_withTimeoutZero(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		ctx, ctxWaitGroup := WithWaitGroup(context.Background())
		ctx, promise := WithFuture(future.WithTimeout(ctx, 0))

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			time.Sleep(100 * time.Millisecond)
			return errors.New("error from worker")
		}).Start(ctx)

		// WithTimeout(0) Get must return immediately
		value, isValid := promise.Get(ctx)
		require.False(t, isValid)
		require.Nil(t, value)

		ctxWaitGroup.Wait()

		// Now the Worker has finished, and a value should have been returned
		value, isValid = promise.Get(ctx)
		require.True(t, isValid)
		require.Error(t, value.(error))
	})
}

func TestWorker_WithFuture_withTimeoutBeforeValue(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		ctx, ctxWaitGroup := WithWaitGroup(context.Background())
		ctx, promise := WithFuture(future.WithTimeout(ctx, 100*time.Millisecond))

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			time.Sleep(200 * time.Millisecond)
			return errors.New("error from worker")
		}).Start(ctx)

		var (
			signalBeforeTimeoutGetterStarted = make(chan struct{}, 1)
			signalTriedValueBeforeTimeout    = make(chan struct{}, 1)
		)
		go func() {
			close(signalBeforeTimeoutGetterStarted)
			value, isValid := promise.Get(ctx)
			require.False(t, isValid)
			require.Nil(t, value)
			close(signalTriedValueBeforeTimeout)
		}()

		<-signalBeforeTimeoutGetterStarted
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-timer.C:
			// just continue
		case <-signalTriedValueBeforeTimeout:
			require.Fail(t, "future getter should not have been returned yet")
		}
		// Now wait for a time that in sum would exceed the timeout set via future.WithTimeout
		timer.Reset(100 * time.Millisecond)
		select {
		case <-timer.C:
			require.Fail(t, "future getter should have been returned already")
		case <-signalTriedValueBeforeTimeout:
			timer.Stop()
			// just continue
		}

		ctxWaitGroup.Wait()

		// Now the Worker has finished, and a valid value should have been set for readout
		value, isValid := promise.Get(ctx)
		require.True(t, isValid)
		require.Error(t, value.(error))
	})
}

func TestWorker_singleRun(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		var (
			ctx, ctxWaitGroup   = WithWaitGroup(context.Background())
			workerInvokeCounter int
		)

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			workerInvokeCounter++
			return nil
		}).Start(ctx)

		ctxWaitGroup.Wait()
		require.Equal(t, 1, workerInvokeCounter)
	})
}

func TestWorker_WithJoinChan(t *testing.T) {
	testtime.MustRunInTime(t, time.Second, func() {
		var (
			ctx, joinC          = WithJoinChan(context.Background())
			workerCalcResult    float64
			signalRunnerStarted = make(chan struct{})
		)

		MakeWorker(t.Name(), func(ctx context.Context) future.Value {
			signalRunnerStarted <- struct{}{}
			workerCalcResult = math.Pow(2, 10)
			return nil
		}).Start(ctx)

		time.Sleep(50 * time.Millisecond)

		require.EqualValues(t, 0, workerCalcResult)
		<-signalRunnerStarted

		<-joinC
		require.EqualValues(t, 1024, workerCalcResult)
	})
}
