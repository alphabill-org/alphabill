package async

import (
	"context"
	"sync"

	"github.com/alphabill-org/alphabill/internal/async/future"
	"github.com/alphabill-org/alphabill/internal/errors"
)

const (
	tplHasWrongType = "{%s} %s has wrong type: %T"
)

type (
	Worker struct {
		name   string
		runner Runner

		config struct {
			waitGroup *sync.WaitGroup
			jointC    JoinChan
			future    *future.Future
		}
	}

	Starter interface {
		Start(context.Context)
	}

	// Runner is the task callback method. Will be invoked only once.
	// Is is up to the user to add an infinite loop for a continues Worker.
	// Must consume ctx.Done signal.
	Runner func(ctx context.Context) future.Value

	// JoinChan is a signal channel in order to wait for a async Worker to finish.
	JoinChan chan struct{}
)

// MakeWorker creates a new Worker instance with the provided runnable object. Runner must be thread-safe.
func MakeWorker(name string, runner Runner) Worker {
	return Worker{
		name:   name,
		runner: runner,
	}
}

// Start initializes a separate go-routine for the worker instance Runner method. The runner can return a future.Value
// (see WithFuture).
//
// Use context.Context for shutdown synchronization.
// For extra context options see contextKey.
func (w Worker) Start(ctx context.Context) {
	ctx, err := w.setup(ctx)
	if err != nil {
		log.Error("{%s} failed to setup: %s", w.name, err)
		return
	}

	go func() {
		var res future.Value
		defer func() {
			if w.config.waitGroup != nil {
				w.config.waitGroup.Done()
			}

			if w.config.jointC != nil {
				close(w.config.jointC)
			}

			if w.config.future != nil {
				w.config.future.Set(res)
			}
		}()

		log.Debug("{%s} Starting...", w.name)
		res = w.runner(ctx)
		log.Info("{%s} ... done!", w.name)
	}()
}

func (w *Worker) setup(ctx context.Context) (context.Context, error) {
	// Check if sync.WaitGroup is provided. Add starting worker to the group.
	if ctxValue := ctx.Value(workerContextWaitGroup); ctxValue != nil {
		switch v := ctxValue.(type) {
		case *sync.WaitGroup:
			w.config.waitGroup = v
			w.config.waitGroup.Add(1)
		default:
			return ctx, errors.Wrapf(errors.ErrInvalidArgument, tplHasWrongType,
				w.name, workerContextWaitGroup, v)
		}
	} else {
		log.Info("{%s} %s not provided!", w.name, workerContextWaitGroup)
	}

	// Check if join channel is set
	if ctxValue := ctx.Value(workerContextJoinChan); ctxValue != nil {
		switch v := ctxValue.(type) {
		case JoinChan:
			w.config.jointC = v
			// The join is meant only for active worker. Set nil for the child workers
			ctx = context.WithValue(ctx, workerContextJoinChan, nil)
		default:
			return ctx, errors.Wrapf(errors.ErrInvalidArgument, tplHasWrongType,
				w.name, workerContextJoinChan, v)
		}
	}

	// Check if future channel is set
	if ctxValue := ctx.Value(workerContextFuture); ctxValue != nil {
		switch v := ctxValue.(type) {
		case *future.Future:
			w.config.future = v
			// The future is meant only for active worker. Set nil for the child workers
			ctx = context.WithValue(ctx, workerContextFuture, nil)
		default:
			return ctx, errors.Wrapf(errors.ErrInvalidArgument, tplHasWrongType,
				w.name, workerContextFuture, v)
		}
	}

	return ctx, nil
}
