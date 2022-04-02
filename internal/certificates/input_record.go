package certificates

import (
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var (
	ErrInputRecordIsNil  = errors.New("input record is nil")
	ErrHashIsNil         = errors.New("hash is nil")
	ErrBlockHashIsNil    = errors.New("block hash is nil")
	ErrPreviousHashIsNil = errors.New("previous hash is nil")
	ErrSummaryValueIsNil = errors.New("summary value is nil")
)

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
	return nil
}

func (x *InputRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.PreviousHash)
	hasher.Write(x.Hash)
	hasher.Write(x.BlockHash)
	hasher.Write(x.SummaryValue)
}
