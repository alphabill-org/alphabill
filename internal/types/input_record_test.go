package types

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

var ir = &InputRecord{
	PreviousHash: []byte{0, 0, 1},
	Hash:         []byte{0, 0, 2},
	BlockHash:    []byte{0, 0, 3},
	SummaryValue: []byte{0, 0, 4},
	RoundNumber:  1,
}

func TestInputRecord_IsValid(t *testing.T) {
	tests := []struct {
		name        string
		inputRecord *InputRecord
		wantErr     error
	}{
		{
			name: "previous hash is nil",
			inputRecord: &InputRecord{
				PreviousHash: nil,
				Hash:         zeroHash,
				BlockHash:    zeroHash,
				SummaryValue: zeroHash,
			},
			wantErr: ErrPreviousHashIsNil,
		},
		{
			name: "hash is nil",
			inputRecord: &InputRecord{
				PreviousHash: zeroHash,
				Hash:         nil,
				BlockHash:    zeroHash,
				SummaryValue: zeroHash,
			},
			wantErr: ErrHashIsNil,
		},
		{
			name: "block hash is nil",
			inputRecord: &InputRecord{
				PreviousHash: zeroHash,
				Hash:         zeroHash,
				BlockHash:    nil,
				SummaryValue: zeroHash,
			},
			wantErr: ErrBlockHashIsNil,
		},
		{
			name: "summary value hash is nil",
			inputRecord: &InputRecord{
				PreviousHash: zeroHash,
				Hash:         zeroHash,
				BlockHash:    zeroHash,
				SummaryValue: nil,
			},
			wantErr: ErrSummaryValueIsNil,
		},
		{
			name: "partition round is 0",
			inputRecord: &InputRecord{
				PreviousHash: zeroHash,
				Hash:         zeroHash,
				BlockHash:    zeroHash,
				SummaryValue: []byte{1, 2, 3},
				RoundNumber:  0,
			},
			wantErr: ErrInvalidPartitionRound,
		},
		{
			name: "valid input record",
			inputRecord: &InputRecord{
				PreviousHash: zeroHash,
				Hash:         zeroHash,
				BlockHash:    zeroHash,
				SummaryValue: zeroHash,
				RoundNumber:  1,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, tt.inputRecord.IsValid())
			} else {
				require.NoError(t, tt.inputRecord.IsValid())
			}
		})
	}
}

func TestInputRecord_IsNil(t *testing.T) {
	var ir *InputRecord
	require.ErrorIs(t, ir.IsValid(), ErrInputRecordIsNil)
}

func TestInputRecord_AddToHasher(t *testing.T) {
	expectedHash, _ := hex.DecodeString("2ce35f17c869ace38d096c92e48a0ca128bdaa1e02ebde88d2bbd2dc9576bb8e")
	hasher := sha256.New()
	ir.AddToHasher(hasher)
	hash := hasher.Sum(nil)
	require.Equal(t, expectedHash, hash)
}
