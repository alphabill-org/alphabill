package wvm

import (
	"github.com/tetratelabs/wazero"
)

type (
	Options struct {
		cfg wazero.RuntimeConfig
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		cfg: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	}
}

func WithRuntimeConfig(cfg wazero.RuntimeConfig) Option {
	return func(c *Options) {
		c.cfg = cfg
	}
}
