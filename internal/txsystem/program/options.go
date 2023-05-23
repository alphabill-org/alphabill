package program

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/rma"
)

var DefaultSmartContractSystemIdentifier = []byte{0, 0, 0, 3}

type (
	Options struct {
		hashAlgorithm crypto.Hash
		state         *rma.Tree
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		hashAlgorithm: crypto.SHA256,
		state:         rma.NewWithSHA256(),
	}
}

func WithState(state *rma.Tree) Option {
	return func(c *Options) {
		c.state = state
	}
}

func WithHashAlgorithm(algorithm crypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}
