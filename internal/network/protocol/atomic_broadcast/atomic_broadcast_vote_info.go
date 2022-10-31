package atomic_broadcast

import (
	"errors"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrInvalidBlockHash    = errors.New("invalid block hash")
	ErrInvalidRootHash     = errors.New("invalid root hash")
	ErrInvalidStateHash    = errors.New("invalid state hash")
	ErrInvalidVoteInfoHash = errors.New("invalid vote info hash")
)

func (x *VoteInfo) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Id)
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	hasher.Write(x.ParentId)
	hasher.Write(util.Uint64ToBytes(x.ParentRound))
	hasher.Write(x.ExecStateId)
}

func (x *VoteInfo) IsValid() error {
	// Todo: epoch is validation rule not yet known
	if x.Round < 1 {
		return ErrInvalidRound
	}
	if len(x.Id) < 1 {
		return ErrInvalidBlockHash
	}
	if len(x.ExecStateId) < 1 {
		return ErrInvalidStateHash
	}
	return nil
}

func (x *CommitInfo) IsValid() error {
	if len(x.VoteInfoHash) < 1 {
		return ErrInvalidVoteInfoHash
	}
	// CommitStateHash can be nil, this is legal
	return nil
}
