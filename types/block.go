package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/tree/mt"
)

var (
	errBlockIsNil             = errors.New("block is nil")
	errBlockHeaderIsNil       = errors.New("block header is nil")
	errPrevBlockHashIsNil     = errors.New("previous block hash is nil")
	errBlockProposerIDMissing = errors.New("block proposer node identifier is missing")
	errTransactionsIsNil      = errors.New("transactions is nil")
	errSystemIDIsNil          = errors.New("system identifier is unassigned")
)

type (
	Block struct {
		_                  struct{} `cbor:",toarray"`
		Header             *Header
		Transactions       []*TransactionRecord
		UnicityCertificate *UnicityCertificate
	}

	Header struct {
		_                 struct{} `cbor:",toarray"`
		SystemID          SystemID
		ShardID           []byte
		ProposerID        string
		PreviousBlockHash []byte
	}
)

// Hash returns the hash of the block. Hash of a block is computed as hash of block header fields and tree hash
// of transactions.
func (b *Block) Hash(algorithm crypto.Hash) ([]byte, error) {
	if err := b.Header.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid block: %w", err)
	}
	if b.UnicityCertificate.GetStateHash() == nil {
		return nil, fmt.Errorf("invalid block: state hash is nil")
	}
	if b.UnicityCertificate.GetPreviousStateHash() == nil {
		return nil, fmt.Errorf("invalid block: previous state hash is nil")
	}
	// 0H - if there are no transactions and state does not change
	if len(b.Transactions) == 0 && bytes.Equal(b.UnicityCertificate.InputRecord.PreviousHash, b.UnicityCertificate.InputRecord.Hash) {
		return make([]byte, algorithm.Size()), nil
	}
	// init transactions merkle root to 0H
	var merkleRoot = make([]byte, algorithm.Size())
	// calculate Merkle tree of transactions if any
	if len(b.Transactions) > 0 {
		// calculate merkle tree root hash from transactions
		tree := mt.New(algorithm, b.Transactions)
		merkleRoot = tree.GetRootHash()
	}
	// header hash || UC.IR.h′ || UC.IR.h || 0H - block Merkle tree root 0H
	headerHash := b.HeaderHash(algorithm)
	hasher := algorithm.New()
	hasher.Write(headerHash)
	hasher.Write(b.UnicityCertificate.InputRecord.PreviousHash)
	hasher.Write(b.UnicityCertificate.InputRecord.Hash)
	hasher.Write(merkleRoot)
	return hasher.Sum(nil), nil
}

func (b *Block) HeaderHash(algorithm crypto.Hash) []byte {
	return b.Header.Hash(algorithm)
}

func (b *Block) GetRoundNumber() uint64 {
	if b != nil {
		return b.UnicityCertificate.GetRoundNumber()
	}
	return 0
}

func (b *Block) GetBlockFees() uint64 {
	if b != nil {
		return b.UnicityCertificate.GetFeeSum()
	}
	return 0
}

func (b *Block) InputRecord() (*InputRecord, error) {
	if b == nil {
		return nil, errBlockIsNil
	}
	if b.UnicityCertificate == nil {
		return nil, errUCIsNil
	}
	if b.UnicityCertificate.InputRecord == nil {
		return nil, ErrInputRecordIsNil
	}
	return b.UnicityCertificate.InputRecord, nil
}

func (b *Block) IsValid() error {
	if b == nil {
		return errBlockIsNil
	}
	if err := b.Header.IsValid(); err != nil {
		return fmt.Errorf("block error: %w", err)
	}
	if b.Transactions == nil {
		return errTransactionsIsNil
	}
	if b.UnicityCertificate == nil {
		return fmt.Errorf("unicity certificate is nil")
	}
	return nil
}

func (b *Block) GetProposerID() string {
	if b == nil || b.Header == nil {
		return ""
	}
	return b.Header.ProposerID
}

func (b *Block) SystemID() SystemID {
	if b == nil || b.Header == nil {
		return 0
	}
	return b.Header.SystemID
}

func (h *Header) Hash(algorithm crypto.Hash) []byte {
	if h == nil {
		return nil
	}
	hasher := algorithm.New()
	hasher.Write(h.SystemID.Bytes())
	hasher.Write(h.ShardID)
	hasher.Write(h.PreviousBlockHash)
	hasher.Write([]byte(h.ProposerID))
	return hasher.Sum(nil)
}

func (h *Header) IsValid() error {
	if h == nil {
		return errBlockHeaderIsNil
	}
	if h.SystemID == 0 {
		return errSystemIDIsNil
	}
	// skip shard identifier for now, it is not used
	if h.PreviousBlockHash == nil {
		return errPrevBlockHashIsNil
	}
	if len(h.ProposerID) == 0 {
		return errBlockProposerIDMissing
	}
	return nil
}
