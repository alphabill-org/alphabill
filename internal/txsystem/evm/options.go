package evm

import (
	gocrypto "crypto"
	"math/big"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
)

type (
	Options struct {
		state                 *rma.Tree
		hashAlgorithm         gocrypto.Hash
		trustBase             map[string]crypto.Verifier
		initialAccountAddress []byte
		initialAccountBalance *big.Int
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		state:                 rma.NewWithSHA256(),
		hashAlgorithm:         gocrypto.SHA256,
		trustBase:             nil,
		initialAccountAddress: make([]byte, 20),
		initialAccountBalance: big.NewInt(100000000000),
	}
}

func WithState(state *rma.Tree) Option {
	return func(c *Options) {
		c.state = state
	}
}

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(tb map[string]crypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = tb
	}
}

func WithInitialAddressAndBalance(address []byte, balance *big.Int) Option {
	return func(o *Options) {
		o.initialAccountAddress = address
		o.initialAccountBalance = balance
	}
}
