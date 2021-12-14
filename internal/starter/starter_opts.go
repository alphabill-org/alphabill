package starter

import (
	"time"
)

type (
	startupOptions struct {
		// Time to wait for component to shutdown before returning an error
		gracefulShutdownTimeout time.Duration
	}

	StartupOption     func(*startupOptions)
	allStartupOptions struct{}
)

var (
	StartupOpts = &allStartupOptions{}
)

func (o *allStartupOptions) GracefulShutdownTimeout(timeout time.Duration) StartupOption {
	return func(options *startupOptions) {
		options.gracefulShutdownTimeout = timeout
	}
}

func aggregateStartupOptions(opts []StartupOption) startupOptions {
	options := startupOptions{
		gracefulShutdownTimeout: 6 * time.Second,
	}

	for _, opt := range opts {
		opt(&options)
	}

	return options
}
