package genesis

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
)

var (
	ErrConsensusParamsIsNil          = errors.New("consensus record is nil")
	ErrInvalidNumberOfRootValidators = errors.New("invalid number of root nodes")
	ErrBlockRateTooSmall             = errors.New("block rate too small")
	ErrUnknownHashAlgorithm          = errors.New("unknown hash algorithm")
	ErrInvalidConsensusTimeout       = errors.New("invalid consensus timeout")
	ErrSignerIsNil                   = errors.New("signer is nil")
	ErrRootValidatorInfoMissing      = errors.New("missing root node public info")
)

const (
	MinBlockRateMs          = 100
	DefaultBlockRateMs      = 900
	MinConsensusTimeout     = 2000
	DefaultConsensusTimeout = 10000
)

type ConsensusParams struct {
	_                   struct{}          `cbor:",toarray"`
	Version             types.ABVersion   `json:"version,omitempty"`
	TotalRootValidators uint32            `json:"total_root_validators,omitempty"` // Number of root validator nodes in the root cluster (1 in case of monolithic root chain)
	BlockRateMs         uint32            `json:"block_rate_ms,omitempty"`         // Block rate (round time t3 in monolithic root chain)
	ConsensusTimeoutMs  uint32            `json:"consensus_timeout_ms,omitempty"`  // Time to abandon proposal and vote for timeout (only used in distributed implementation)
	HashAlgorithm       uint32            `json:"hash_algorithm,omitempty"`        // Hash algorithm for UnicityTree calculation
	Signatures          map[string][]byte `json:"signatures,omitempty"`            // Signed hash of all fields excluding signatures
}

func (x *ConsensusParams) IsValid() error {
	if x == nil {
		return ErrConsensusParamsIsNil
	}
	if x.Version == 0 {
		return types.ErrInvalidVersion(x)
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
	// Timeout must be bigger than round min block-rate+2s
	if x.BlockRateMs+MinConsensusTimeout > x.ConsensusTimeoutMs {
		return fmt.Errorf("invalid timeout for block rate, must be at least %d", x.BlockRateMs+MinConsensusTimeout)
	}
	if hashAlgo := gocrypto.Hash(x.HashAlgorithm); !hashAlgo.Available() {
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
	b.Write(util.Uint32ToBytes(x.GetVersion()))
	b.Write(util.Uint32ToBytes(x.TotalRootValidators))
	b.Write(util.Uint32ToBytes(x.BlockRateMs))
	b.Write(util.Uint32ToBytes(x.ConsensusTimeoutMs))
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

func (x *ConsensusParams) GetVersion() types.ABVersion {
	return x.Version
}

func (x *ConsensusParams) MarshalCBOR() ([]byte, error) {
	type alias ConsensusParams
	return types.Cbor.MarshalTaggedValue(types.ConsensusParamsTag, (*alias)(x))
}

func (x *ConsensusParams) UnmarshalCBOR(data []byte) error {
	type alias ConsensusParams
	return types.Cbor.UnmarshalTaggedValue(types.ConsensusParamsTag, data, (*alias)(x))
}
