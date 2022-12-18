package atomic_broadcast

import (
	"crypto"
	"reflect"
	"testing"

	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/stretchr/testify/require"
)

func TestLedgerCommitInfo_Bytes(t *testing.T) {
	x := &LedgerCommitInfo{
		VoteInfoHash:  []byte{0, 1, 2, 3},
		CommitStateId: []byte{4, 5, 6, 7},
	}
	bytes := x.Bytes()
	require.Equal(t, len(bytes), len(x.VoteInfoHash)+len(x.CommitStateId))
	require.Equal(t, bytes, []byte{0, 1, 2, 3, 4, 5, 6, 7})
}

func TestLedgerCommitInfo_Hash(t *testing.T) {
	x := &LedgerCommitInfo{
		VoteInfoHash:  []byte{0, 1, 2, 3},
		CommitStateId: []byte{4, 5, 6, 7},
	}
	h := x.Hash(crypto.SHA256)
	require.Equal(t, crypto.SHA256.Size(), len(h))
	require.Equal(t, h, abhash.Sum256([]byte{0, 1, 2, 3, 4, 5, 6, 7}))
}

func TestLedgerCommitInfo_IsValid(t *testing.T) {
	type fields struct {
		VoteInfoHash  []byte
		CommitStateId []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name:    "valid",
			fields:  fields{VoteInfoHash: abhash.Sum256([]byte("test")), CommitStateId: []byte{1, 2, 3}},
			wantErr: nil,
		},
		{
			name:    "valid, commit state can be nil",
			fields:  fields{VoteInfoHash: abhash.Sum256([]byte("test")), CommitStateId: nil},
			wantErr: nil,
		},
		{
			name:    "not-valid",
			fields:  fields{VoteInfoHash: nil, CommitStateId: []byte{1, 2, 3}},
			wantErr: ErrInvalidVoteInfoHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &LedgerCommitInfo{
				VoteInfoHash:  tt.fields.VoteInfoHash,
				CommitStateId: tt.fields.CommitStateId,
			}
			require.ErrorIs(t, x.IsValid(), tt.wantErr)
		})
	}
}

func TestVoteInfo_Hash(t *testing.T) {
	type fields struct {
		RootRound   uint64
		Epoch       uint64
		Timestamp   uint64
		ParentRound uint64
		ExecStateId []byte
	}
	type args struct {
		hash crypto.Hash
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []byte
	}{
		{
			name: "compare hash",
			args: args{hash: crypto.SHA256},
			fields: fields{
				RootRound:   23,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: []byte{0, 1, 2, 5},
			},
			// hash of buffer with all fields in the order (integers are interpreted as big endian 8 byte values)
			want: abhash.Sum256([]byte{
				0, 0, 0, 0, 0, 0, 0, 23,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 11,
				0, 0, 0, 0, 0, 0, 0, 22,
				0, 1, 2, 5}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteInfo{
				RootRound:   tt.fields.RootRound,
				Epoch:       tt.fields.Epoch,
				Timestamp:   tt.fields.Timestamp,
				ParentRound: tt.fields.ParentRound,
				ExecStateId: tt.fields.ExecStateId,
			}
			if got := x.Hash(tt.args.hash); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Hash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVoteInfo_IsValid(t *testing.T) {
	type fields struct {
		RootRound   uint64
		Epoch       uint64
		Timestamp   uint64
		ParentRound uint64
		ExecStateId []byte
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
				RootRound:   23,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: []byte{0, 1, 2, 5},
			},
			wantErr: false,
			errIs:   nil,
		},
		{
			name: "Invalid round number",
			fields: fields{
				RootRound:   0,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: []byte{0, 1, 2, 5},
			},
			wantErr: true,
			errIs:   ErrInvalidRound,
		},
		{
			name: "Parent round must be strictly smaller than current round",
			fields: fields{
				RootRound:   22,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: []byte{0, 1, 2, 5},
			},
			errIs: ErrInvalidRound,
		},
		{
			name: "excec state id is nil",
			fields: fields{
				RootRound:   23,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: nil,
			},
			errIs: ErrInvalidStateHash,
		},
		{
			name: "excec state id is empty",
			fields: fields{
				RootRound:   23,
				Epoch:       0,
				Timestamp:   11,
				ParentRound: 22,
				ExecStateId: []byte{},
			},
			errIs: ErrInvalidStateHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteInfo{
				RootRound:   tt.fields.RootRound,
				Epoch:       tt.fields.Epoch,
				Timestamp:   tt.fields.Timestamp,
				ParentRound: tt.fields.ParentRound,
				ExecStateId: tt.fields.ExecStateId,
			}
			err := x.IsValid()
			require.ErrorIs(t, err, tt.errIs)
		})
	}
}
