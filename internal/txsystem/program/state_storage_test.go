package program

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func decodeID(t *testing.T, hexStr string) []byte {
	t.Helper()
	b, err := hex.DecodeString(hexStr)
	require.NoError(t, err)
	return b
}

func TestExecIDToStateFileID(t *testing.T) {
	type args struct {
		id      *uint256.Int
		stateID []byte
	}
	tests := []struct {
		name string
		args args
		want *uint256.Int
	}{
		{
			name: "generate state id",
			args: args{
				id:      uint256.NewInt(1),
				stateID: []byte{1, 2, 3, 4},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "3D459E0FCF22CFCD1D7EED46BCFA68193C009BEC3D53CBE69E5997632CCF6F54")),
		},
		{
			name: "works with any number of bytes",
			args: args{
				id:      uint256.NewInt(1),
				stateID: []byte{1, 2, 3},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "9184ABD2BB318731D717E972057240EAE26CCA202A8D35DBE9D2176F526886A0")),
		},
		{
			name: "file id is 0",
			args: args{
				id:      uint256.NewInt(1),
				stateID: []byte{0, 0, 0, 0},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "957B88B12730E646E0F33D3618B77DFA579E8231E3C59C7104BE7165611C8027")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CreateStateFileID(tt.args.id, tt.args.stateID); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateStateFileID() = %X, want %X", got.Bytes32(), tt.want.Bytes32())
			}
		})
	}
}

func TestNewStateStorage_Fails(t *testing.T) {
	s, err := NewStateStorage(nil, nil)
	require.ErrorContains(t, err, "state tree is nil")
	require.Nil(t, s)

	s, err = NewStateStorage(rma.NewWithSHA256(), nil)
	require.ErrorContains(t, err, "program execution context is nil")
	require.Nil(t, s)
}
func TestNewStateStorage(t *testing.T) {
	eCtx := &ExecutionContext{
		programID:   counterProgramUnitID,
		progParams:  uint64ToLEBytes(0),
		inputParams: uint64ToLEBytes(1),
	}
	s, err := NewStateStorage(rma.NewWithSHA256(), eCtx)
	buf, err := s.Read([]byte("test"))
	require.ErrorContains(t, err, rma.ErrUnitNotFound.Error())
	// add key
	require.NoError(t, s.Write([]byte("test"), []byte{1, 2, 3}))
	// read key
	buf, err = s.Read([]byte("test"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(buf, []byte{1, 2, 3}))
	// update key
	require.NoError(t, s.Write([]byte("test"), []byte{2, 3, 4}))
	buf, err = s.Read([]byte("test"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(buf, []byte{2, 3, 4}))
}
