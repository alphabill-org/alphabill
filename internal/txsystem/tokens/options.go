package tokens

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
)

var DefaultTokenTxSystemIdentifier = []byte{0, 0, 0, 2}

type (
	Options struct {
		systemIdentifier []byte
		hashAlgorithm    gocrypto.Hash
		trustBase        map[string]crypto.Verifier
		state            TokenState
	}

	Option func(*Options)
)

func defaultOptions() (*Options, error) {
	hashAlgorithm := gocrypto.SHA256
	state, err := rma.New(&rma.Config{
		HashAlgorithm: hashAlgorithm,
	})
	if err != nil {
		return nil, err
	}

	return &Options{
		systemIdentifier: DefaultTokenTxSystemIdentifier,
		hashAlgorithm:    hashAlgorithm,
		state:            state,
	}, nil
}

func WithState(state TokenState) Option {
	return func(c *Options) {
		c.state = state
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

func WithTrustBase(trustBase map[string]crypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = trustBase
	}
}
