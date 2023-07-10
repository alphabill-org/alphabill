package txsystem

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/state"
)

type Options struct {
	systemIdentifier    []byte
	hashAlgorithm       crypto.Hash
	state               *state.State
	beginBlockFunctions []TxEmitter
	endBlockFunctions   []TxEmitter
	systemGeneratedTxs  map[string]bool
}

type Option func(*Options)

func DefaultOptions() *Options {
	return &Options{
		hashAlgorithm:       crypto.SHA256,
		state:               state.NewEmptyState(),
		beginBlockFunctions: make([]TxEmitter, 0),
		endBlockFunctions:   make([]TxEmitter, 0),
		systemGeneratedTxs:  make(map[string]bool),
	}
}

func WithSystemGeneratedTxs(txs map[string]bool) Option {
	return func(g *Options) {
		for k, v := range txs {
			g.systemGeneratedTxs[k] = v
		}
	}
}

func WithBeginBlockFunctions(funcs []TxEmitter) Option {
	return func(g *Options) {
		g.beginBlockFunctions = append(g.beginBlockFunctions, funcs...)
	}
}

func WithEndBlockFunctions(funcs []TxEmitter) Option {
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
