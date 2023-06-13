package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrRoundCreationTimeNotSet  = errors.New("round creation time not set")
	ErrRootInfoInvalidRound     = errors.New("root round info round number is not valid")
	ErrInvalidRootRoundInfoHash = errors.New("root round info latest root hash is not valid")
)

type RoundInfo struct {
	_                 struct{} `cbor:",toarray"`
	RoundNumber       uint64   `json:"root_chain_round_number,omitempty"`
	Epoch             uint64   `json:"root_epoch,omitempty"`
	Timestamp         uint64   `json:"timestamp,omitempty"`
	ParentRoundNumber uint64   `json:"root_chain_parent_round_number,omitempty"`
	CurrentRootHash   []byte   `json:"current_root_hash,omitempty"`
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

func (x *RoundInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.Bytes())
	return hasher.Sum(nil)
}

func (x *RoundInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.RoundNumber))
	b.Write(util.Uint64ToBytes(x.Epoch))
	b.Write(util.Uint64ToBytes(x.Timestamp))
	b.Write(util.Uint64ToBytes(x.ParentRoundNumber))
	b.Write(x.CurrentRootHash)
	return b.Bytes()
}

func (x *RoundInfo) IsValid() error {
	if x.RoundNumber < 1 || x.RoundNumber <= x.ParentRoundNumber {
		return ErrRootInfoInvalidRound
	}
	if len(x.CurrentRootHash) < 1 {
		return ErrInvalidRootRoundInfoHash
	}
	if x.Timestamp == 0 {
		return ErrRoundCreationTimeNotSet
	}
	return nil
}
