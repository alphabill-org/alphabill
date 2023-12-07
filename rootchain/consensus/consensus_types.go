package consensus

import (
	gocrypto "crypto"
	"time"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
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
	}

	Option func(c *Optional)
)

// NewConsensusParams extract common consensus parameters from genesis
func NewConsensusParams(genesisRoot *genesis.GenesisRootRecord) *Parameters {
	return &Parameters{
		BlockRate:          time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeout:       time.Duration(genesisRoot.Consensus.ConsensusTimeoutMs) * time.Millisecond,
		ConsensusThreshold: genesisRoot.Consensus.QuorumThreshold,
		HashAlgorithm:      gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm),
	}
}

func WithStorage(db keyvaluedb.KeyValueDB) Option {
	return func(c *Optional) {
		c.Storage = db
	}
}

func LoadConf(opts []Option) *Optional {
	conf := &Optional{
		Storage: memorydb.New(),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}
