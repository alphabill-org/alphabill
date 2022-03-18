package txsystem

import (
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestSameShardId(t *testing.T) {
	emptyBytes32 := [32]byte{}
	randomHash := test.RandomBytes(32)
	randomId := test.RandomBytes(32)
	type args struct {
		id        *uint256.Int
		hashValue []byte
	}
	tests := []struct {
		name string
		args args
		want *uint256.Int
	}{
		{
			name: "empty",
			args: args{
				id:        uint256.NewInt(0),
				hashValue: []byte{},
			},
			want: uint256.NewInt(0).SetBytes(emptyBytes32[:]),
		}, {
			name: "random",
			args: args{
				id:        uint256.NewInt(0).SetBytes(randomId),
				hashValue: randomHash,
			},
			want: uint256.NewInt(0).SetBytes(append(randomId[:4], randomHash[4:]...)),
		}, {
			name: "id empty",
			args: args{
				id:        uint256.NewInt(0),
				hashValue: randomHash,
			},
			want: uint256.NewInt(0).SetBytes(copyAndAppend(emptyBytes32[:4], randomHash[4:]...)),
		}, {
			name: "hash empty",
			args: args{
				id:        uint256.NewInt(0).SetBytes(randomId),
				hashValue: []byte{},
			},
			want: uint256.NewInt(0).SetBytes(copyAndAppend(randomId[:4], emptyBytes32[4:]...)),
		}, {
			name: "hash half empty",
			args: args{
				id:        uint256.NewInt(0).SetBytes(randomId),
				hashValue: randomHash[:16],
			},
			want: uint256.NewInt(0).SetBytes(
				copyAndAppend(randomId[:4],
					copyAndAppend(randomHash[4:16], emptyBytes32[16:]...)...,
				)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantBytes := tt.want.Bytes32()
			actual := SameShardId(tt.args.id, tt.args.hashValue)
			actualBytes := actual.Bytes32()
			assert.Equalf(t, tt.want, actual, "sameShardId(%v, %v)", tt.args.id, tt.args.hashValue)
			assert.Equal(t, wantBytes, actualBytes)
		})
	}
}

// normal append modified the slice appended too
func copyAndAppend(i []byte, vals ...byte) []byte {
	j := make([]byte, len(i), len(i)+len(vals))
	copy(j, i)
	return append(j, vals...)
}
