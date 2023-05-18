package txsystem

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/rma"
)

type Options struct {
	systemIdentifier    []byte
	hashAlgorithm       crypto.Hash
	state               *rma.Tree
	beginBlockFunctions []func(blockNumber uint64)
	endBlockFunctions   []func(blockNumber uint64) error
}

type Option func(*Options)

func DefaultOptions() *Options {
	return &Options{
		hashAlgorithm:       crypto.SHA256,
		state:               rma.NewWithSHA256(),
		beginBlockFunctions: make([]func(blockNumber uint64), 0),
		endBlockFunctions:   make([]func(blockNumber uint64) error, 0),
	}
}

func WithBeginBlockFunctions(funcs []func(blockNumber uint64)) Option {
	return func(g *Options) {
		g.beginBlockFunctions = funcs
	}
}

func WithEndBlockFunctions(funcs []func(blockNumber uint64) error) Option {
	return func(g *Options) {
		g.endBlockFunctions = funcs
	}
}

func WithSystemIdentifier(systemID []byte) Option {
	return func(g *Options) {
		g.systemIdentifier = systemID
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithState(s *rma.Tree) Option {
	return func(g *Options) {
		g.state = s
	}
}
