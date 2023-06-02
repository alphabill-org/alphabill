package program

import (
	"bytes"
	"crypto/sha256"
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
	programID := sha256.Sum256([]byte("test"))
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
				id:      uint256.NewInt(0).SetBytes(programID[:]),
				stateID: []byte{1, 2, 3, 4},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "9F86D081E1B97F131FABB6B447296C9B6F0201E79FB3C5356E6C77E89B6A806A")),
		},
		{
			name: "works with any number of bytes",
			args: args{
				id:      uint256.NewInt(0).SetBytes(programID[:]),
				stateID: []byte{1, 2, 3},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "9F86D081F2C0CB492C533B0A4D14EF77CC0F78ABCCCED5287D84A1A2011CFB81")),
		},
		{
			name: "file id is 0",
			args: args{
				id:      uint256.NewInt(0).SetBytes(programID[:]),
				stateID: []byte{0, 0, 0, 0},
			},
			want: uint256.NewInt(0).SetBytes(decodeID(t, "9F86D08104A92FDB4057192DC43DD748EA778ADC52BC498CE80524C014B81119")),
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
