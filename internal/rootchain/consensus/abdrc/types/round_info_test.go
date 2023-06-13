package types

import (
	gocrypto "crypto"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestRoundInfo_Bytes(t *testing.T) {
	x := &RoundInfo{
		RoundNumber:       4,
		Epoch:             1,
		Timestamp:         0x12345678,
		ParentRoundNumber: 3,
		CurrentRootHash:   make([]byte, 32),
	}
	serialized := make([]byte, 0, 4*8+32)
	serialized = append(serialized, util.Uint64ToBytes(x.RoundNumber)...)
	serialized = append(serialized, util.Uint64ToBytes(x.Epoch)...)
	serialized = append(serialized, util.Uint64ToBytes(x.Timestamp)...)
	serialized = append(serialized, util.Uint64ToBytes(x.ParentRoundNumber)...)
	serialized = append(serialized, x.CurrentRootHash...)
	buf := x.Bytes()
	require.True(t, reflect.DeepEqual(buf, serialized))
}

func TestRoundInfo_GetParentRound(t *testing.T) {
	var x *RoundInfo = nil
	require.EqualValues(t, 0, x.GetRound())
	x = &RoundInfo{ParentRoundNumber: 3}
	require.EqualValues(t, 3, x.GetParentRound())
}

func TestRoundInfo_GetRound(t *testing.T) {
	var x *RoundInfo = nil
	require.EqualValues(t, 0, x.GetRound())
	x = &RoundInfo{RoundNumber: 3}
	require.EqualValues(t, 3, x.GetRound())
}

func TestRoundInfo_Hash(t *testing.T) {
	x := &RoundInfo{
		RoundNumber:       4,
		Epoch:             1,
		Timestamp:         0x12345678,
		ParentRoundNumber: 3,
		CurrentRootHash:   make([]byte, 32),
	}
	require.NotNil(t, x.Hash(gocrypto.SHA256))
}

func TestRootRoundInfo_IsValid(t *testing.T) {
	type fields struct {
		RoundNumber       uint64
		Epoch             uint64
		Timestamp         uint64
		ParentRoundNumber uint64
		CurrentRootHash   []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
		errIs   error
	}{
		{
			name: "valid",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{0, 1, 2, 5},
			},
			wantErr: false,
			errIs:   nil,
		},
		{
			name: "Parent round must be strictly smaller than current round",
			fields: fields{
				RoundNumber:       22,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{0, 1, 2, 5},
			},
			wantErr: true,
			errIs:   ErrRootInfoInvalidRound,
		},
		{
			name: "current root hash is nil",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   nil,
			},
			wantErr: true,
			errIs:   ErrInvalidRootRoundInfoHash,
		},
		{
			name: "current root hash is empty",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{},
			},
			wantErr: true,
			errIs:   ErrInvalidRootRoundInfoHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RoundInfo{
				RoundNumber:       tt.fields.RoundNumber,
				Epoch:             tt.fields.Epoch,
				Timestamp:         tt.fields.Timestamp,
				ParentRoundNumber: tt.fields.ParentRoundNumber,
				CurrentRootHash:   tt.fields.CurrentRootHash,
			}
			if err := x.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
