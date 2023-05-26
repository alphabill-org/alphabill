package program

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

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
			want: uint256.NewInt(0).SetBytes([]byte{
				1, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 1, 2, 3, 4,
			}),
		},
		{
			name: "error - invalid state id",
			args: args{
				id:      uint256.NewInt(1),
				stateID: []byte{1, 2, 3},
			},
			want: uint256.NewInt(0).SetBytes([]byte{
				1, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 1, 2, 3,
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CreateStateFileID(tt.args.id, tt.args.stateID); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateStateFileID() = %v, want %v", got, tt.want)
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
