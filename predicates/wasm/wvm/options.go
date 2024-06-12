package wvm

import (
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/tetratelabs/wazero"
)

type (
	Options struct {
		cfg     wazero.RuntimeConfig
		storage keyvaluedb.KeyValueDB
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	memDB, _ := memorydb.New()
	return &Options{
		cfg:     wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
		storage: memDB,
	}
}

func WithRuntimeConfig(cfg wazero.RuntimeConfig) Option {
	return func(c *Options) {
		c.cfg = cfg
	}
}

func WithStorage(db keyvaluedb.KeyValueDB) Option {
	return func(c *Options) {
		c.storage = db
	}
}
