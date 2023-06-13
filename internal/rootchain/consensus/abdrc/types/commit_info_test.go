package types

import (
	"bytes"
	gocrypto "crypto"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestCommitInfo_Bytes(t *testing.T) {
	commitInfo := &CommitInfo{
		RootRoundInfoHash: []byte{0, 1, 2},
		RootHash:          []byte{2, 3, 4},
		Round:             1,
		Epoch:             0,
		Timestamp:         1668208271000, // 11.11.2022 @ 11:11:11
	}
	var serialized []byte
	serialized = append(serialized, commitInfo.RootRoundInfoHash...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Round)...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Epoch)...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Timestamp)...)
	serialized = append(serialized, commitInfo.RootHash...)
	cInfoBytes := commitInfo.Bytes()
	require.True(t, reflect.DeepEqual(serialized, cInfoBytes))
}

func TestCommitInfo_Hash(t *testing.T) {
	commitInfo := &CommitInfo{
		RootRoundInfoHash: []byte{0, 1, 2},
		RootHash:          []byte{2, 3, 4},
		Round:             1,
		Epoch:             0,
		Timestamp:         1668208271000, // 11.11.2022 @ 11:11:11
	}
	// make sure hash is calculated over commit info serialized like this
	var serialized []byte
	serialized = append(serialized, commitInfo.RootRoundInfoHash...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Round)...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Epoch)...)
	serialized = append(serialized, util.Uint64ToBytes(commitInfo.Timestamp)...)
	serialized = append(serialized, commitInfo.RootHash...)
	hasher := gocrypto.SHA256.New()
	hasher.Write(serialized)
	require.True(t, bytes.Equal(commitInfo.Hash(gocrypto.SHA256), hasher.Sum(nil)))
}

func TestCommitInfo_IsValid(t *testing.T) {
	type fields struct {
		Round             uint64
		RootRoundInfoHash []byte
		RootHash          []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name:   "valid",
			fields: fields{RootRoundInfoHash: hash.Sum256([]byte("test")), Round: 1, RootHash: []byte{1, 2, 3}},
		},
		{
			name:    "invalid, round is 0",
			fields:  fields{RootRoundInfoHash: hash.Sum256([]byte("test")), Round: 0, RootHash: []byte{1, 2, 3}},
			wantErr: errInvalidRootRound,
		},
		{
			name:    "invalid, root hash is nil",
			fields:  fields{RootRoundInfoHash: hash.Sum256([]byte("test")), Round: 1, RootHash: nil},
			wantErr: errRootStateHashIsMissing,
		},
		{
			name:    "invalid, root hash len is 0",
			fields:  fields{RootRoundInfoHash: hash.Sum256([]byte("test")), Round: 1, RootHash: []byte{}},
			wantErr: errRootStateHashIsMissing,
		},
		{
			name:    "invalid, root round info hash is nil",
			fields:  fields{RootRoundInfoHash: nil, Round: 1, RootHash: []byte{1, 2, 3}},
			wantErr: errInvalidRoundInfoHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &CommitInfo{
				RootRoundInfoHash: tt.fields.RootRoundInfoHash,
				Round:             tt.fields.Round,
				RootHash:          tt.fields.RootHash,
			}
			err := x.IsValid()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
