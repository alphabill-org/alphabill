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

func (x *BlockInfo) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(x.Id)
	hasher.Write(x.RootHash)
	hasher.Write(x.StateHash)
}

func (x *BlockInfo) IsValid() error {
	// Todo: epoch is validation rule not yet known
	if x.Round < 1 {
		return ErrInvalidRound
	}
	if len(x.Id) < 1 {
		return ErrInvalidBlockHash
	}
	if len(x.RootHash) < 1 {
		return ErrInvalidRootHash
	}
	if len(x.StateHash) < 1 {
		return ErrInvalidStateHash
	}
	return nil
}

func (x *VoteInfo) IsValid() error {
	if err := x.Proposed.IsValid(); err != nil {
		return err
	}
	if err := x.Parent.IsValid(); err != nil {
		return err
	}
	return nil
}

func (x *VoteInfo) AddToHasher(hasher hash.Hash) {
	x.Proposed.AddToHasher(hasher)
	x.Parent.AddToHasher(hasher)
}

func (x *CommitInfo) IsValid() error {
	if len(x.VoteInfoHash) < 1 {
		return ErrInvalidVoteInfoHash
	}
	// CommitStateHash can be nil, this is legal
	return nil
}
