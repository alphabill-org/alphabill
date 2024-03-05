package types

import (
	"bytes"
	"errors"
	"fmt"
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

func EqualIR(a, b *InputRecord) bool {
	return bytes.Equal(a.Bytes(), b.Bytes())
}

func AssertEqualIR(a, b *InputRecord) error {
	if a.RoundNumber != b.RoundNumber {
		return fmt.Errorf("round number is different: %v vs %v", a.RoundNumber, b.RoundNumber)
	}
	if a.SumOfEarnedFees != b.SumOfEarnedFees {
		return fmt.Errorf("sum of fees is different: %v vs %v", a.SumOfEarnedFees, b.SumOfEarnedFees)
	}
	if !bytes.Equal(a.SummaryValue, b.SummaryValue) {
		return fmt.Errorf("summary value is different: %v vs %v", a.SummaryValue, b.SummaryValue)
	}
	if !bytes.Equal(a.PreviousHash, b.PreviousHash) {
		return fmt.Errorf("previous state hash is different: %X vs %X", a.PreviousHash, b.PreviousHash)
	}
	if !bytes.Equal(a.Hash, b.Hash) {
		return fmt.Errorf("state hash is different: %X vs %X", a.Hash, b.Hash)
	}
	if !bytes.Equal(a.BlockHash, b.BlockHash) {
		return fmt.Errorf("block hash is different: %X vs %X", a.BlockHash, b.BlockHash)
	}
	return nil
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
	if isZeroHash(x.BlockHash) {
		if !bytes.Equal(x.PreviousHash, x.Hash) {
			return fmt.Errorf("block hash is 0H, but state hash changes")
		}
	}
	if bytes.Equal(x.PreviousHash, x.Hash) {
		if !isZeroHash(x.BlockHash) {
			return fmt.Errorf("state hash does not change, but block hash is 0H")
		}
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

func (x *InputRecord) String() string {
	if x == nil {
		return "input record is nil"
	}
	return fmt.Sprintf("H: %X H': %X Bh: %X round: %v fees: %d summary: %d",
		x.Hash, x.PreviousHash, x.BlockHash, x.RoundNumber, x.SumOfEarnedFees, x.SumOfEarnedFees)
}
