package cli

import (
	"strings"
	"time"
)

type (
	startupOptions struct {
		// Time to wait for component to shutdown before returning an error
		gracefulShutdownTimeout time.Duration
	}

	cliOptions struct {
		envPrefix string
	}
)

var (
	StartupOpts = &allStartupOptions{}
	Opts        = &allCliOptions{}
)

type StartupOption func(*startupOptions)
type Option func(*cliOptions)

type allStartupOptions struct{}
type allCliOptions struct{}

func (o *allStartupOptions) GracefulShutdownTimeout(timeout time.Duration) StartupOption {
	return func(options *startupOptions) {
		options.gracefulShutdownTimeout = timeout
	}
}

func (o *allCliOptions) EnvironmentVariablePrefix(prefix string) Option {
	return func(options *cliOptions) {
		options.envPrefix = prefix
	}
}

func aggregateCliOptions(componentName string, opts []Option) cliOptions {
	options := cliOptions{
		envPrefix: "AB",
	}
	if len(componentName) > 1 {
		options.envPrefix = strings.ToUpper("AB_" + componentName)
	}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

func aggregateStartupOptions(opts []StartupOption) startupOptions {
	options := startupOptions{
		gracefulShutdownTimeout: 6 * time.Second,
	}

	for _, opt := range opts {
		opt(&options)
	}

	if FlagGracefulExitTimeout != nil && *FlagGracefulExitTimeout > 0 {
		log.Debug("Overwriting graceful exit timeout: %s", *FlagGracefulExitTimeout)
		options.gracefulShutdownTimeout = *FlagGracefulExitTimeout
	}

	return options
}
