package genesis

import (
	"bytes"
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrConsensusParamsIsNil          = errors.New("consensus record is nil")
	ErrInvalidNumberOfRootValidators = errors.New("invalid number of root validators")
	ErrConsensusNotSigned            = errors.New("consensus struct is not signed")
	ErrBlockRateTooSmall             = errors.New("block rate too small")
	ErrInvalidQuorumThreshold        = errors.New("invalid quorum threshold")
	ErrUnknownHashAlgorithm          = errors.New("unknown hash algorithm")
	ErrInvalidConsensusTimeout       = errors.New("invalid consensus timeout")
	ErrConsensusUnknownSigner        = errors.New("consensus unknown signer")
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
	// Exception to return something useful for monolithic case
	if totalRootValidators == 1 {
		return 1
	}
	// Otherwise, if there are less than min total validators, return 0
	if totalRootValidators < MinDistributedRootValidators {
		return 0
	}
	// Calculate
	// total nodes in the system N=3f+1, hence compromised can be up to f=(N-1)/3
	faultTolerance := (totalRootValidators - 1) / 3
	// Quorum is achieved by Q=2f+1 nodes
	return (2 * faultTolerance) + 1
}

func (x *ConsensusParams) IsValid() error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if x.TotalRootValidators < 1 {
		return ErrInvalidNumberOfRootValidators
	}
	// There are two configurations that are supported:
	// 1. monolithic root chain - has exactly 1 root validator
	// 2. distributed root chain - must have at least MinDistributedRootValidators
	if x.TotalRootValidators > 1 && x.TotalRootValidators < MinDistributedRootValidators {
		return ErrInvalidNumberOfRootValidators
	}
	// depending on configuration, distributed root chain may never be this fast, but it must not be faster
	if x.BlockRateMs < MinBlockRateMs {
		return ErrBlockRateTooSmall
	}
	// If defined:  validate consensus timeout (only used in distributed set-up)
	if x.ConsensusTimeoutMs != nil {
		if *x.ConsensusTimeoutMs < x.BlockRateMs {
			return ErrInvalidConsensusTimeout
		}
	}
	// If defined: verify quorum threshold
	if x.QuorumThreshold != nil {
		// Therefore, the defined quorum threshold must be same or higher
		if *x.QuorumThreshold < GetMinQuorumThreshold(x.TotalRootValidators) {
			return ErrInvalidQuorumThreshold
		}
		if *x.QuorumThreshold > x.TotalRootValidators {
			return ErrInvalidQuorumThreshold
		}
	}
	hashAlgo := gocrypto.Hash(x.HashAlgorithm)
	if hashAlgo.Available() == false {
		return ErrUnknownHashAlgorithm
	}
	return nil
}

func (x *ConsensusParams) Bytes() []byte {
	var b bytes.Buffer
	// self is nil?
	if x == nil {
		return b.Bytes()
	}
	b.Write(util.Uint32ToBytes(x.TotalRootValidators))
	b.Write(util.Uint32ToBytes(x.BlockRateMs))
	if x.ConsensusTimeoutMs != nil {
		b.Write(util.Uint32ToBytes(*x.ConsensusTimeoutMs))
	}
	if x.QuorumThreshold != nil {
		b.Write(util.Uint32ToBytes(*x.QuorumThreshold))
	}
	b.Write(util.Uint32ToBytes(x.HashAlgorithm))
	return b.Bytes()
}

func (x *ConsensusParams) Sign(id string, signer crypto.Signer) error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if signer == nil {
		return certificates.ErrSignerIsNil
	}
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return err
	}
	// initiate signatures
	if x.Signatures == nil {
		x.Signatures = make(map[string][]byte)
	}
	x.Signatures[id] = signature
	return nil
}

func (x *ConsensusParams) Verify(verifiers map[string]crypto.Verifier) error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if verifiers == nil {
		return certificates.ErrRootValidatorInfoMissing
	}
	if len(x.Signatures) == 0 {
		return ErrConsensusNotSigned
	}
	// Verify all signatures, all must be from known origin and valid
	for id, sig := range x.Signatures {
		// Find verifier info
		ver, f := verifiers[id]
		if !f {
			return ErrConsensusUnknownSigner
		}
		err := ver.VerifyBytes(sig, x.Bytes())
		if err != nil {
			return errors.Wrap(err, "invalid consensus signature")
		}
	}
	return nil
}
