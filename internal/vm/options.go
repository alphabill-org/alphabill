package vm

import "github.com/tetratelabs/wazero"

type (
	Options struct {
		storage Storage
		cfg     wazero.RuntimeConfig
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		storage: NewMemoryStorage(),
		cfg:     wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	}
}

func WithStorage(s Storage) Option {
	return func(c *Options) {
		c.storage = s
	}
}

func WithRuntimeConfig(cfg wazero.RuntimeConfig) Option {
	return func(c *Options) {
		c.cfg = cfg
	}
}
