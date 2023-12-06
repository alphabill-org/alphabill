package state

import (
	"crypto"
	"io"
)

type (
	Options struct {
		hashAlgorithm       crypto.Hash
		actions             []Action
		reader              io.Reader
		unitDataConstructor UnitDataConstructor
	}

	Option func(o *Options)
)

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(o *Options) {
		o.hashAlgorithm = hashAlgorithm
	}
}

func WithInitActions(actions ...Action) Option {
	return func(o *Options) {
		o.actions = actions
	}
}

func WithReader(reader io.Reader) Option {
	return func(o *Options) {
		o.reader = reader
	}
}

func WithUnitDataConstructor(unitDataConstructor UnitDataConstructor) Option {
	return func(o *Options) {
		o.unitDataConstructor = unitDataConstructor
	}
}

func loadOptions(opts ...Option) *Options {
	options := &Options{
		hashAlgorithm: crypto.SHA256,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}
