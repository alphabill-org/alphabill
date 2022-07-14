package async

import (
	"context"
	"sync"

	"github.com/alphabill-org/alphabill/internal/async/future"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/logger"
)

var log = logger.CreateForPackage()

type (
	// contextKey is used for setting context.WithValue options
	contextKey string
)

const (
	// workerContextWaitGroup is for providing *sync.WaitGroup value via context.Context for Worker shutdown
	// synchronization.
	// A new Worker will increase the delta value of the set sync.WaitGroup and invoke Done on exit.
	// The user must only wait for the all workers to finish with WaitGroup.Wait.
	workerContextWaitGroup = contextKey("workerContextWaitGroup")
	// workerContextJoinChan if used, a join channel will be set up for the user to be able to join worker.
	// When worker is finished the returned channel is closed.
	workerContextJoinChan = contextKey("workerContextJointChan")
	// workerContextFuture sets up a context with a future.Future. The value is returned after runner has completed
	workerContextFuture = contextKey("workerContextFuture")
)

// WithWaitGroup sets and returns a wait group.
//
// Note that the set wait group is propagated to all child routines.
func WithWaitGroup(parent context.Context) (context.Context, *sync.WaitGroup) {
	var wg sync.WaitGroup
	return context.WithValue(parent, workerContextWaitGroup, &wg), &wg
}

// WaitGroup returns the set wait group
func WaitGroup(ctx context.Context) (*sync.WaitGroup, error) {
	ctxValue := ctx.Value(workerContextWaitGroup)
	if ctxValue == nil {
		return nil, errors.Wrap(errors.ErrInvalidState, "wait group not set")
	}

	switch v := ctxValue.(type) {
	case *sync.WaitGroup:
		return v, nil
	default:
		return nil, errors.Wrapf(errors.ErrInvalidArgument, "value has incorrect type: %T", v)
	}
}

// WithJoinChan returns a join channel in order for the parent routine to wait for the child to finish.
//
// Note that the set channel is only available for the first child routine, meaning it will not propagate recursively
// to lover level child routines. However, the child routine can setup its own join channel.
func WithJoinChan(parent context.Context) (context.Context, JoinChan) {
	var joinC = make(JoinChan)
	return context.WithValue(parent, workerContextJoinChan, joinC), joinC
}

// WithFuture return a future for reading a returned value from the child routine.
//
// Note that the set future is only available for the first child routine, meaning it will not propagate recursively
// to lover level child routines. However, the child routine can setup its own future.
func WithFuture(parent context.Context) (context.Context, *future.Future) {
	var f = future.New()
	return context.WithValue(parent, workerContextFuture, f), f
}
