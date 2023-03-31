package genesis

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrConsensusParamsIsNil          = errors.New("consensus record is nil")
	ErrInvalidNumberOfRootValidators = errors.New("invalid number of root nodes")
	ErrConsensusNotSigned            = errors.New("consensus struct is not signed")
	ErrBlockRateTooSmall             = errors.New("block rate too small")
	ErrUnknownHashAlgorithm          = errors.New("unknown hash algorithm")
	ErrInvalidConsensusTimeout       = errors.New("invalid consensus timeout")
	ErrSignerIsNil                   = errors.New("signer is nil")
	ErrRootValidatorInfoMissing      = errors.New("missing root node public info")
	ErrConsensusIsNotSignedByAll     = errors.New("consensus is not signed by all root nodes")
)

const (
	MinBlockRateMs          = 500
	DefaultBlockRateMs      = 900
	MinConsensusTimeout     = 2000
	DefaultConsensusTimeout = 10000
)

// GetMinQuorumThreshold calculated minimal quorum threshold from total number of validators
// Returns 0 if threshold cannot be calculated. Either because it is a monolithic root chain
// or there are less than MinDistributedRootValidators in total
func GetMinQuorumThreshold(totalRootValidators uint32) uint32 {
	// must be over 2/3
	// +1 to round up and avoid using floats
	return (totalRootValidators*2)/3 + 1
}

func (x *ConsensusParams) IsValid() error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if x.TotalRootValidators < 1 {
		return ErrInvalidNumberOfRootValidators
	}
	// depending on configuration, distributed root chain may never be this fast, but it must not be faster
	if x.BlockRateMs < MinBlockRateMs {
		return ErrBlockRateTooSmall
	}
	// If defined:  validate consensus timeout (only used in distributed set-up)
	if x.ConsensusTimeoutMs < MinConsensusTimeout {
		return ErrInvalidConsensusTimeout
	}
	// Therefore, the defined quorum threshold must be same or higher
	minQuorum := GetMinQuorumThreshold(x.TotalRootValidators)
	if x.QuorumThreshold < minQuorum {
		return fmt.Errorf("quorum threshold set too low %v, must be at least %v",
			x.QuorumThreshold, minQuorum)
	}
	if x.QuorumThreshold > x.TotalRootValidators {
		return fmt.Errorf("quorum threshold set higher %v than number of validators in root chain %v",
			x.QuorumThreshold, x.TotalRootValidators)
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
	b.Write(util.Uint32ToBytes(x.ConsensusTimeoutMs))
	b.Write(util.Uint32ToBytes(x.QuorumThreshold))
	b.Write(util.Uint32ToBytes(x.HashAlgorithm))
	return b.Bytes()
}

func (x *ConsensusParams) Sign(id string, signer crypto.Signer) error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if signer == nil {
		return ErrSignerIsNil
	}
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return fmt.Errorf("failed to sign consensus params %w", err)
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
		return ErrRootValidatorInfoMissing
	}
	// If there are more signatures, then we will give more detailed info below on what id is missing
	if len(x.Signatures) < len(verifiers) {
		return fmt.Errorf("consensus parameters is not signed by all validators, validators %v/signatures %v",
			len(verifiers), len(x.Signatures))
	}
	// Verify all signatures, all must be from known origin and valid
	for id, sig := range x.Signatures {
		// Find verifier info
		ver, f := verifiers[id]
		if !f {
			return fmt.Errorf("consensus parameters signed by unknown validator: %v", id)
		}
		err := ver.VerifyBytes(sig, x.Bytes())
		if err != nil {
			return fmt.Errorf("consensus parameters signature verification error: %w", err)
		}
	}
	return nil
}
