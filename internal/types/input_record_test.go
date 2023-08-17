package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

var ir = &InputRecord{
	PreviousHash:    []byte{0, 0, 1},
	Hash:            []byte{0, 0, 2},
	BlockHash:       []byte{0, 0, 3},
	SummaryValue:    []byte{0, 0, 4},
	RoundNumber:     1,
	SumOfEarnedFees: 20,
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
	expectedHash, _ := hex.DecodeString("c8a1b4ed8f753eddc73762e9666ba4012e99d44633ee4576153a31d2f03385b4")
	hasher := sha256.New()
	ir.AddToHasher(hasher)
	hash := hasher.Sum(nil)
	require.Equal(t, expectedHash, hash)
}

func Test_InputRecord_Equal(t *testing.T) {
	var irA = &InputRecord{
		PreviousHash:    []byte{1, 1, 1},
		Hash:            []byte{2, 2, 2},
		BlockHash:       []byte{3, 3, 3},
		SummaryValue:    []byte{4, 4, 4},
		RoundNumber:     2,
		SumOfEarnedFees: 33,
	}

	tests := []struct {
		name string
		ir   *InputRecord
		want bool
	}{
		{
			name: "equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     2,
				SumOfEarnedFees: 33,
			},
			want: true,
		},
		{
			name: "Previous hash not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     2,
				SumOfEarnedFees: 33,
			},
			want: false,
		},
		{
			name: "Hash not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2, 3},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     2,
				SumOfEarnedFees: 33,
			},
			want: false,
		},
		{
			name: "Block hash not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       nil,
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     2,
				SumOfEarnedFees: 33,
			},
			want: false,
		},
		{
			name: "Summary value not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{},
				RoundNumber:     2,
				SumOfEarnedFees: 33,
			},
			want: false,
		},
		{
			name: "RoundNumber not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     1,
				SumOfEarnedFees: 33,
			},
			want: false,
		},
		{
			name: "SumOfEarnedFees not equal",
			ir: &InputRecord{
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     2,
				SumOfEarnedFees: 1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := irA.Equal(tt.ir); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInputRecord_NewRepeatUC(t *testing.T) {
	repeatUC := ir.NewRepeatIR()
	require.NotNil(t, repeatUC)
	// only round number is advance by 1
	ir.RoundNumber++
	require.True(t, bytes.Equal(ir.Bytes(), repeatUC.Bytes()))
	require.True(t, reflect.DeepEqual(ir, repeatUC))
}
