package util

import (
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestSameShardId(t *testing.T) {
	emptyBytes32 := [32]byte{}
	randomHash := test.RandomBytes(32)
	randomId := test.RandomBytes(32)
	type args struct {
		id        types.UnitID
		hashValue []byte
	}
	tests := []struct {
		name string
		args args
		want types.UnitID
	}{
		{
			name: "empty",
			args: args{
				id:        util.Uint256ToBytes(uint256.NewInt(0)),
				hashValue: []byte{},
			},
			want: util.Uint256ToBytes(uint256.NewInt(0).SetBytes(emptyBytes32[:])),
		}, {
			name: "random",
			args: args{
				id:        randomId,
				hashValue: randomHash,
			},
			want: util.Uint256ToBytes(uint256.NewInt(0).SetBytes(append(randomId[:4], randomHash[4:]...))),
		}, {
			name: "id empty",
			args: args{
				id:        util.Uint256ToBytes(uint256.NewInt(0)),
				hashValue: randomHash,
			},
			want: util.Uint256ToBytes(uint256.NewInt(0).SetBytes(copyAndAppend(emptyBytes32[:4], randomHash[4:]...))),
		}, {
			name: "hash empty",
			args: args{
				id:        util.Uint256ToBytes(uint256.NewInt(0).SetBytes(randomId)),
				hashValue: []byte{},
			},
			want: util.Uint256ToBytes(uint256.NewInt(0).SetBytes(copyAndAppend(randomId[:4], emptyBytes32[4:]...))),
		}, {
			name: "hash half empty",
			args: args{
				id:        util.Uint256ToBytes(uint256.NewInt(0).SetBytes(randomId)),
				hashValue: randomHash[:16],
			},
			want: util.Uint256ToBytes(uint256.NewInt(0).SetBytes(
				copyAndAppend(randomId[:4],
					copyAndAppend(randomHash[4:16], emptyBytes32[16:]...)...,
				))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SameShardID(tt.args.id, tt.args.hashValue)
			assert.Equalf(t, tt.want, actual, "sameShardId(%v, %v)", tt.args.id, tt.args.hashValue)
		})
	}
}

// normal append modified the slice appended too
func copyAndAppend(i []byte, vals ...byte) []byte {
	j := make([]byte, len(i), len(i)+len(vals))
	copy(j, i)
	return append(j, vals...)
}
