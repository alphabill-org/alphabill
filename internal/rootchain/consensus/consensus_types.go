package consensus

import (
	"context"
	gocrypto "crypto"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type (
	// Manager is a consensus manager interface that must be implemented by all versions of consensus managers
	Manager interface {
		// RequestCertification returns channel where to send certification requests with proof of quorum or no-quorum
		RequestCertification() chan<- IRChangeRequest
		// CertificationResult read the channel to receive certification results
		CertificationResult() <-chan certificates.UnicityCertificate
		// GetLatestUnicityCertificate get the latest certification for partition (maybe should/can be removed)
		GetLatestUnicityCertificate(id protocol.SystemIdentifier) (*certificates.UnicityCertificate, error)
		// Run consensus algorithm
		Run(ctx context.Context) error
	}

	// Parameters are basic consensus parameters that need to be the same in all root validators.
	// Extracted from root genesis where all validators in the root cluster must have signed them to signal agreement
	Parameters struct {
		BlockRateMs        time.Duration // also known as T3
		LocalTimeoutMs     time.Duration
		ConsensusThreshold uint32
		HashAlgorithm      gocrypto.Hash
	}
	// Optional are common optional parameters for consensus managers
	Optional struct {
		StoragePath string
	}

	Option func(c *Optional)
)

// NewConsensusParams extract common consensus parameters from genesis
func NewConsensusParams(genesisRoot *genesis.GenesisRootRecord) *Parameters {
	return &Parameters{
		BlockRateMs:        time.Duration(genesisRoot.Consensus.BlockRateMs) * time.Millisecond,
		LocalTimeoutMs:     time.Duration(genesisRoot.Consensus.ConsensusTimeoutMs) * time.Millisecond,
		ConsensusThreshold: genesisRoot.Consensus.QuorumThreshold,
		HashAlgorithm:      gocrypto.Hash(genesisRoot.Consensus.HashAlgorithm),
	}
}

func WithPersistentStoragePath(s string) Option {
	return func(c *Optional) {
		c.StoragePath = s
	}
}

func LoadConf(opts []Option) *Optional {
	conf := &Optional{
		StoragePath: "",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}
