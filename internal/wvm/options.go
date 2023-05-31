package wvm

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type (
	Options struct {
		cfg     wazero.RuntimeConfig
		hostMod []HostModuleFn
	}

	Option       func(*Options)
	HostModuleFn func(context.Context, wazero.Runtime) (api.Module, error)
)

func defaultOptions() *Options {
	return &Options{
		cfg:     wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
		hostMod: make([]HostModuleFn, 0, 2),
	}
}

func WithRuntimeConfig(cfg wazero.RuntimeConfig) Option {
	return func(c *Options) {
		c.cfg = cfg
	}
}

func WithHostModule(hostFn HostModuleFn) Option {
	return func(c *Options) {
		c.hostMod = append(c.hostMod, hostFn)
	}
}
