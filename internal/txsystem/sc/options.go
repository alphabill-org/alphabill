package sc

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/state"
)

var DefaultSmartContractSystemIdentifier = []byte{0, 0, 0, 3}

type (
	Options struct {
		hashAlgorithm crypto.Hash
		state         *state.State
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		hashAlgorithm: crypto.SHA256,
		state:         state.NewEmptyState(),
	}
}

func WithState(state *state.State) Option {
	return func(c *Options) {
		c.state = state
	}
}

func WithHashAlgorithm(algorithm crypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}
