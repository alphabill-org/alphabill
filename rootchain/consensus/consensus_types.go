package consensus

import (
	"crypto"
	"time"
)

const (
	BlockRate     = 900
	LocalTimeout  = 10000
	HashAlgorithm = crypto.SHA256
)

type (
	// Parameters are basic consensus parameters that need to be the same in all root validators.
	// Extracted from root genesis where all validators in the root cluster must have signed them to signal agreement
	Parameters struct {
		BlockRate          time.Duration // also known as T3
		LocalTimeout       time.Duration
		ConsensusThreshold uint32
		HashAlgorithm      crypto.Hash
	}
	// Optional are common optional parameters for consensus managers
	Optional struct {
		Params *Parameters
	}

	Option func(c *Optional)
)

func NewConsensusParams() *Parameters {
	return &Parameters{
		BlockRate:     time.Duration(BlockRate) * time.Millisecond,
		LocalTimeout:  time.Duration(LocalTimeout) * time.Millisecond,
		HashAlgorithm: HashAlgorithm,
	}
}

func WithConsensusParams(params Parameters) Option {
	return func(c *Optional) {
		c.Params = &params
	}
}

func LoadConf(opts []Option) (*Optional, error) {
	conf := &Optional{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}

	if conf.Params == nil {
		conf.Params = NewConsensusParams()
	}

	return conf, nil
}
