package future

import (
	"context"
	"time"
)

type getterContextKey string

const (
	getterContextWithTimeout getterContextKey = "getterContextWithTimeout"
)

// WithTimeout applies a timeout for the future value getter.
func WithTimeout(parent context.Context, timeout time.Duration) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithValue(parent, getterContextWithTimeout, timeout)
}
