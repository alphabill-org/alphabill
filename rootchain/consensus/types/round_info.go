package types

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

var (
	errRoundNumberUnassigned   = errors.New("round number is not assigned")
	errParentRoundUnassigned   = errors.New("parent round number is not assigned")
	errRootHashUnassigned      = errors.New("root hash is not assigned")
	errRoundCreationTimeNotSet = errors.New("round creation time is not set")
)

type RoundInfo struct {
	_                 struct{} `cbor:",toarray"`
	Version           types.ABVersion
	RoundNumber       uint64    `json:"rootChainRoundNumber"`
	Epoch             uint64    `json:"rootEpoch"`
	Timestamp         uint64    `json:"timestamp"`
	ParentRoundNumber uint64    `json:"rootChainParentRoundNumber"`
	CurrentRootHash   hex.Bytes `json:"currentRootHash"`
}

func (x *RoundInfo) GetRound() uint64 {
	if x != nil {
		return x.RoundNumber
	}
	return 0
}

func (x *RoundInfo) GetParentRound() uint64 {
	if x != nil {
		return x.ParentRoundNumber
	}
	return 0
}

func (x *RoundInfo) Hash(hash gocrypto.Hash) ([]byte, error) {
	hasher := abhash.New(hash.New())
	hasher.Write(x)
	return hasher.Sum()
}

func (x *RoundInfo) IsValid() error {
	if x.RoundNumber == 0 {
		return errRoundNumberUnassigned
	}
	if x.ParentRoundNumber == 0 && x.RoundNumber > 1 {
		return errParentRoundUnassigned
	}
	if x.RoundNumber <= x.ParentRoundNumber {
		return fmt.Errorf("invalid round number %d - must be greater than parent round %d", x.RoundNumber, x.ParentRoundNumber)
	}
	if len(x.CurrentRootHash) < 1 {
		return errRootHashUnassigned
	}
	if x.Timestamp == 0 {
		return errRoundCreationTimeNotSet
	}
	return nil
}

func (x *RoundInfo) GetVersion() types.ABVersion {
	if x == nil || x.Version == 0 {
		return 1
	}
	return x.Version
}

func (x *RoundInfo) MarshalCBOR() ([]byte, error) {
	type alias RoundInfo
	if x.Version == 0 {
		x.Version = x.GetVersion()
	}
	return types.Cbor.MarshalTaggedValue(types.RootPartitionRoundInfoTag, (*alias)(x))
}

func (x *RoundInfo) UnmarshalCBOR(data []byte) error {
	type alias RoundInfo
	if err := types.Cbor.UnmarshalTaggedValue(types.RootPartitionRoundInfoTag, data, (*alias)(x)); err != nil {
		return err
	}
	return types.EnsureVersion(x, x.Version, 1)
}
