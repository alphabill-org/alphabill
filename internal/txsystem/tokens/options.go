package tokens

import gocrypto "crypto"

var DefaultTokenTxSystemIdentifier = []byte{0, 0, 0, 2}

type (
	Options struct {
		systemIdentifier []byte
		hashAlgorithm    gocrypto.Hash
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		systemIdentifier: DefaultTokenTxSystemIdentifier,
		hashAlgorithm:    gocrypto.SHA256,
	}
}

func WithSystemIdentifier(systemIdentifier []byte) Option {
	return func(c *Options) {
		c.systemIdentifier = systemIdentifier
	}
}

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}
