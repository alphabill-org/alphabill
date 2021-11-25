package future

import (
	"context"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

type (
	Future struct {
		c        chan Value
		value    Value
		received bool
		err      error
		setOnce  sync.Once
	}

	Value interface{}
)

// New is Future constructor.
func New() *Future {
	return &Future{
		c: make(chan Value, 1),
	}
}

// Set sets the future value, witch can be read/extracted via Get.
//
// Note that a value can only be set once. Any sequential attempts will be ignored
func (f *Future) Set(v Value) {
	if f == nil || f.c == nil {
		return
	}
	f.setOnce.Do(func() {
		f.c <- v
		close(f.c)
	})
}

// Get returns the future Value along with status flag, whether the returned value is valid (has been set).
// Note that a nil Value might be also valid.
//
// Get blocks by default until a value has been set. However, WithTimeout can be used to modify the behaviour,
// as following:
//  - in case of a negative timeout value, Get will block as if the context wouldn't have been modified;
//  - in case the set timeout is 0 (zero), Get will return immediately no matter whether a Value has been set,
//    or not;
//  - in case of a positive timeout value, Get will return on a set future Value or when the timeout expires (depending
//    on witch one is triggered first);
//
// See Future.Err for more info in case the status is set to false.
func (f *Future) Get(ctx context.Context) (Value, bool) {
	if f == nil || f.c == nil {
		return nil, false
	}
	// Check if a value has been already set
	if f.received {
		return f.value, f.received
	}

	var (
		timeout time.Duration = -1
	)
	if ctxValue := ctx.Value(getterContextWithTimeout); ctxValue != nil {
		timeout = ctxValue.(time.Duration)
	}

	switch {
	case timeout < 0:
		// Block until a value has been received, or context has been cancelled
		select {
		case <-ctx.Done():
			f.err = ctx.Err()
			// do nothing, just continue (parent context has been cancelled)
		case value := <-f.c:
			f.setReceived(value)
		}
	case timeout == 0:
		// Check whether a value has been set
		select {
		case value := <-f.c:
			f.setReceived(value)
		default:
			f.err = errors.ErrTimeout
			// do nothing, just continue (value ha not been set yet)
		}
	default:
		// Wait until timeout.
		timer := time.NewTimer(timeout)
		select {
		case <-timer.C:
			f.err = errors.ErrTimeout
			// do nothing, just continue (user set deadline has passed)
		case <-ctx.Done():
			timer.Stop()
			f.err = ctx.Err()
			// do nothing, just continue (parent context has been cancelled)
		case value := <-f.c:
			timer.Stop()
			f.setReceived(value)
		}
	}
	return f.value, f.received
}

// Err returns an error that has occurred while value readout.
func (f *Future) Err() error {
	return f.err
}

func (f *Future) setReceived(value Value) {
	f.value = value
	f.received = true
}

// C returns the channel on which the future value is to be set.
// Use instead of Get in case a more controlled select clause needs to be implemented.
//
// Note that a value can be extracted only once (compared to Get where the set value can be read multiple times).
// After a value has been extracted, the channel is closed.
func (f *Future) C() <-chan Value {
	if f == nil {
		return nil
	}
	return f.c
}
