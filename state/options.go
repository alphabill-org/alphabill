package state

import (
	"crypto"
)

type (
	Options struct {
		hashAlgorithm crypto.Hash
	}

	Option func(o *Options)
)

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(o *Options) {
		o.hashAlgorithm = hashAlgorithm
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
