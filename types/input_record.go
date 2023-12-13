package types

import (
	"bytes"
	"errors"
	"hash"

	"github.com/alphabill-org/alphabill/util"
)

var (
	ErrInputRecordIsNil      = errors.New("input record is nil")
	ErrHashIsNil             = errors.New("hash is nil")
	ErrBlockHashIsNil        = errors.New("block hash is nil")
	ErrPreviousHashIsNil     = errors.New("previous hash is nil")
	ErrSummaryValueIsNil     = errors.New("summary value is nil")
	ErrInvalidPartitionRound = errors.New("partition round is 0")
)

type InputRecord struct {
	_               struct{} `cbor:",toarray"`
	PreviousHash    []byte   `json:"previous_hash,omitempty"`      // previously certified state hash
	Hash            []byte   `json:"hash,omitempty"`               // state hash to be certified
	BlockHash       []byte   `json:"block_hash,omitempty"`         // hash of the block
	SummaryValue    []byte   `json:"summary_value,omitempty"`      // summary value to certified
	RoundNumber     uint64   `json:"round_number,omitempty"`       // transaction system's round number
	SumOfEarnedFees uint64   `json:"sum_of_earned_fees,omitempty"` // sum of the actual fees over all transaction records in the block
}

func isZeroHash(hash []byte) bool {
	for _, b := range hash {
		if b != 0 {
			return false
		}
	}
	return true
}

func (x *InputRecord) Equal(b *InputRecord) bool {
	return x.RoundNumber == b.RoundNumber &&
		x.SumOfEarnedFees == b.SumOfEarnedFees &&
		bytes.Equal(x.PreviousHash, b.PreviousHash) &&
		bytes.Equal(x.Hash, b.Hash) &&
		bytes.Equal(x.BlockHash, b.BlockHash) &&
		bytes.Equal(x.SummaryValue, b.SummaryValue)
}

func (x *InputRecord) IsValid() error {
	if x == nil {
		return ErrInputRecordIsNil
	}
	if x.Hash == nil {
		return ErrHashIsNil
	}
	if x.BlockHash == nil {
		return ErrBlockHashIsNil
	}
	if x.PreviousHash == nil {
		return ErrPreviousHashIsNil
	}
	if x.SummaryValue == nil {
		return ErrSummaryValueIsNil
	}
	if x.RoundNumber == 0 {
		return ErrInvalidPartitionRound
	}
	return nil
}

func (x *InputRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *InputRecord) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.PreviousHash)
	b.Write(x.Hash)
	b.Write(x.BlockHash)
	b.Write(x.SummaryValue)
	b.Write(util.Uint64ToBytes(x.RoundNumber))
	b.Write(util.Uint64ToBytes(x.SumOfEarnedFees))
	return b.Bytes()
}

// NewRepeatIR - creates new repeat IR from current IR
func (x *InputRecord) NewRepeatIR() *InputRecord {
	return &InputRecord{
		PreviousHash:    bytes.Clone(x.PreviousHash),
		Hash:            bytes.Clone(x.Hash),
		BlockHash:       bytes.Clone(x.BlockHash),
		SummaryValue:    bytes.Clone(x.SummaryValue),
		RoundNumber:     x.RoundNumber,
		SumOfEarnedFees: x.SumOfEarnedFees,
	}
}
