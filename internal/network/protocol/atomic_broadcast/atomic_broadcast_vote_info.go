package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
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

func NewCommitInfo(commitStateHash []byte, voteInfo *VoteInfo, hash gocrypto.Hash) *LedgerCommitInfo {
	hasher := hash.New()
	voteInfo.AddToHasher(hasher)
	return &LedgerCommitInfo{CommitStateId: commitStateHash, VoteInfoHash: hasher.Sum(nil)}
}

func (x *LedgerCommitInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.CommitStateId)
	b.Write(x.VoteInfoHash)
	return b.Bytes()
}

func (x *LedgerCommitInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.CommitStateId)
	hasher.Write(x.VoteInfoHash)
	return hasher.Sum(nil)
}

func (x *LedgerCommitInfo) IsValid() error {
	if len(x.VoteInfoHash) < 1 {
		return ErrInvalidVoteInfoHash
	}
	// CommitStateId can be nil, this is legal
	return nil
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
	if x.RootRound < 1 {
		return ErrInvalidRound
	}
	if len(x.BlockId) < 1 {
		return ErrInvalidBlockHash
	}
	if len(x.ExecStateId) < 1 {
		return ErrInvalidStateHash
	}
	return nil
}
