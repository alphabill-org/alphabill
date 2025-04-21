package consensus

import (
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
)

const (
	BlockRate     = 900
	LocalTimeout  = 10000
	HashAlgorithm = gocrypto.SHA256
)

type (
	// Parameters are basic consensus parameters that need to be the same in all root validators.
	// Extracted from root genesis where all validators in the root cluster must have signed them to signal agreement
	Parameters struct {
		BlockRate          time.Duration // also known as T3
		LocalTimeout       time.Duration
		ConsensusThreshold uint32
		HashAlgorithm      gocrypto.Hash
	}
	// Optional are common optional parameters for consensus managers
	Optional struct {
		Storage keyvaluedb.KeyValueDB
		Params  *Parameters
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

func WithStorage(db keyvaluedb.KeyValueDB) Option {
	return func(c *Optional) {
		c.Storage = db
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

	if conf.Storage == nil {
		var err error
		if conf.Storage, err = memorydb.New(); err != nil {
			return nil, fmt.Errorf("creating storage: %w", err)
		}
	}
	return conf, nil
}
