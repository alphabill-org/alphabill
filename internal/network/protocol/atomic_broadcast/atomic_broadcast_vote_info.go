package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrInvalidBlockId      = errors.New("invalid block id")
	ErrInvalidStateHash    = errors.New("invalid state hash")
	ErrInvalidVoteInfoHash = errors.New("invalid vote info hash")
)

func (x *LedgerCommitInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.VoteInfoHash)
	b.Write(x.CommitStateId)
	return b.Bytes()
}

func (x *LedgerCommitInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.VoteInfoHash)
	hasher.Write(x.CommitStateId)
	return hasher.Sum(nil)
}

func (x *LedgerCommitInfo) IsValid() error {
	if len(x.VoteInfoHash) < 1 {
		return ErrInvalidVoteInfoHash
	}
	// CommitStateId can be nil, this is legal
	return nil
}

func (x *VoteInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	x.AddToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *VoteInfo) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.BlockId)
	hasher.Write(util.Uint64ToBytes(x.RootRound))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	hasher.Write(x.ParentBlockId)
	hasher.Write(util.Uint64ToBytes(x.ParentRound))
	hasher.Write(x.ExecStateId)
}

func (x *VoteInfo) IsValid() error {
	// Todo: epoch is validation rule not yet known
	if x.RootRound < 1 || x.RootRound <= x.ParentRound {
		return ErrInvalidRound
	}
	if len(x.BlockId) < 1 {
		return ErrInvalidBlockId
	}
	if len(x.ExecStateId) < 1 {
		return ErrInvalidStateHash
	}
	return nil
}
