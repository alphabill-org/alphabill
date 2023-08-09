package txsystem

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/state"
)

type Options struct {
	systemIdentifier    []byte
	hashAlgorithm       crypto.Hash
	state               *state.State
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
}

type Option func(*Options)

func DefaultOptions() *Options {
	return &Options{
		hashAlgorithm:       crypto.SHA256,
		state:               state.NewEmptyState(),
		beginBlockFunctions: make([]func(blockNumber uint64) error, 0),
		endBlockFunctions:   make([]func(blockNumber uint64) error, 0),
	}
}

func WithBeginBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) {
		g.beginBlockFunctions = append(g.beginBlockFunctions, funcs...)
	}
}

func WithEndBlockFunctions(funcs ...func(blockNumber uint64) error) Option {
	return func(g *Options) {
		g.endBlockFunctions = append(g.endBlockFunctions, funcs...)
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

func WithState(s *state.State) Option {
	return func(g *Options) {
		g.state = s
	}
}
