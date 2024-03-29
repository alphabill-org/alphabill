package txsystem

import (
	"crypto"

	"github.com/alphabill-org/alphabill/state"
)

type Options struct {
	hashAlgorithm       crypto.Hash
	state               *state.State
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
}

type Option func(*Options)

func DefaultOptions() *Options {
	return &Options{
		hashAlgorithm: crypto.SHA256,
		state:         state.NewEmptyState(),
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
