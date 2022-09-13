package genesis

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
	"hash"
)

var (
	ErrConsensusParamsIsNil          = errors.New("system description record is nil")
	ErrInvalidNumberOfRootValidators = errors.New("invalid number of root validators")
	ErrConsensusNotSigned            = errors.New("consensus struct is not signed")
)

const (
	// MinDistributedRootValidators defines min number of distributed root chain validators.
	// Total number of root validators is defined by N=3f+1
	// If at least one faulty/compromised validator is to be tolerated then min nodes is 3*1+1=4
	MinDistributedRootValidators = 4
	MinBlockRateMs               = 900
)

// GetMinQuorumThreshold calculated minimal quorum threshold from total number of validators
// Returns 0 if threshold cannot be calculated. Either because it is a monolithic root chain
// or there are less than MinDistributedRootValidators in total
func GetMinQuorumThreshold(totalRootValidators uint32) uint32 {
	if totalRootValidators < MinDistributedRootValidators {
		return 0
	}
	// total nodes in the system N=3f+1, hence compromised can be up to f=(N-1)/3
	faultTolerance := (totalRootValidators - 1) / 3
	// Quorum is achieved by Q=2f+1 nodes
	return (2 * faultTolerance) - 1
}

func (x *ConsensusParams) IsValid() error {
	if x == nil {
		return ErrSystemDescriptionIsNil
	}

	if x.TotalRootValidators < 1 {
		return ErrInvalidNumberOfRootValidators
	}
	if len(x.Signatures) == 0 {
		return ErrConsensusNotSigned
	}
	// There are two configurations that are supported:
	// 1. monolithic root chain - has exactly 1 root validator
	// 2. distributed root chain - must have at least MinDistributedRootValidators
	if x.TotalRootValidators > 1 && x.TotalRootValidators < MinDistributedRootValidators {
		return ErrInvalidNumberOfRootValidators
	}
	// depending on configuration, distributed root chain may never be this fast, but it must not be faster
	if x.BlockRateMs < MinBlockRateMs {
		return errors.New("Block rate too small < 900")
	}
	// If defined:  validate consensus timeout (only used in distributed set-up)
	if x.ConsensusTimeoutMs != nil {
		if *x.ConsensusTimeoutMs < x.BlockRateMs {
			return errors.New("Consensus timeout is defined smaller than block rate")
		}
	}
	// If defined: verify quorum threshold
	if x.QuorumThreshold != nil {
		// Therefore, the defined quorum threshold must be same or higher
		if *x.QuorumThreshold < GetMinQuorumThreshold(x.TotalRootValidators) {
			return errors.New("Invalid quorum threshold")
		}
	}
	hashAlgo := gocrypto.Hash(x.HashAlgorithm)
	if hashAlgo.Available() == false {
		return errors.New("Unknown hash algorithm")
	}
	return nil
}

func (x *ConsensusParams) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(x.TotalRootValidators)))
	hasher.Write(util.Uint64ToBytes(uint64(x.BlockRateMs)))
	if x.ConsensusTimeoutMs != nil {
		hasher.Write(util.Uint64ToBytes(uint64(*x.ConsensusTimeoutMs)))
	}
	if x.QuorumThreshold != nil {
		hasher.Write(util.Uint64ToBytes(uint64(*x.QuorumThreshold)))
	}
	hasher.Write(util.Uint64ToBytes(uint64(x.HashAlgorithm)))
}

func (x *ConsensusParams) Hash(hashAlgorithm gocrypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddToHasher(hasher)
	return hasher.Sum(nil)
}
