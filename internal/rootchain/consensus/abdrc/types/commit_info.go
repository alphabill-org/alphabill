package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	errInvalidRoundInfoHash   = errors.New("round info has is missing")
	errRootStateHashIsMissing = errors.New("missing or invalid root state hash")
	errInvalidRootRound       = errors.New("invalid root round number")
)

type CommitInfo struct {
	_                 struct{} `cbor:",toarray"`
	RootRoundInfoHash []byte   `json:"root_round_info_hash"`
	Round             uint64   `json:"committed_root_round"`
	Epoch             uint64   `json:"epoch"`
	Timestamp         uint64   `json:"timestamp"`
	RootHash          []byte   `json:"root_hash"`
}

func NewCommitInfo(algo gocrypto.Hash, info RoundInfo) *CommitInfo {
	return &CommitInfo{
		RootRoundInfoHash: info.Hash(algo),
		Round:             info.RoundNumber,
		Epoch:             info.Epoch,
		Timestamp:         info.Timestamp,
		RootHash:          info.CurrentRootHash,
	}
}

func (x *CommitInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.RootRoundInfoHash)
	b.Write(util.Uint64ToBytes(x.Round))
	b.Write(util.Uint64ToBytes(x.Epoch))
	b.Write(util.Uint64ToBytes(x.Timestamp))
	b.Write(x.RootHash)
	return b.Bytes()
}

func (x *CommitInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.RootRoundInfoHash)
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	hasher.Write(x.RootHash)
	return hasher.Sum(nil)
}

func (x *CommitInfo) IsValid() error {
	if len(x.RootRoundInfoHash) < 1 {
		return errInvalidRoundInfoHash
	}
	if len(x.RootHash) < 1 {
		return errRootStateHashIsMissing
	}
	if x.Round < 1 {
		return errInvalidRootRound
	}
	return nil
}
