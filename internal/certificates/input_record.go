package certificates

import (
	"bytes"
	"errors"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrInputRecordIsNil             = errors.New("input record is nil")
	ErrHashIsNil                    = errors.New("hash is nil")
	ErrBlockHashIsNil               = errors.New("block hash is nil")
	ErrPreviousHashIsNil            = errors.New("previous hash is nil")
	ErrSummaryValueIsNil            = errors.New("summary value is nil")
	ErrInvalidPartitionRound        = errors.New("partition round is 0")
	ErrInvalidZeroBlockChangesState = errors.New("zero block hash, changes state hash")
)

func isZeroHash(hash []byte) bool {
	for _, b := range hash {
		if b != 0 {
			return false
		}
	}
	return true
}

func NewRepeatInputRecord(lastIR *InputRecord) (*InputRecord, error) {
	if lastIR == nil {
		return nil, ErrInputRecordIsNil
	}
	return &InputRecord{
		PreviousHash:    lastIR.Hash,
		Hash:            lastIR.Hash,
		BlockHash:       make([]byte, len(lastIR.BlockHash)),
		SummaryValue:    lastIR.SummaryValue,
		RoundNumber:     lastIR.RoundNumber + 1,
		SumOfEarnedFees: lastIR.SumOfEarnedFees,
	}, nil
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
