package util

import (
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/assert"
)

const (
	typeIDLength = 1
	unitIDLength = 32 + typeIDLength
	typeIDIndex  = unitIDLength - typeIDLength
)

func TestSameShardId(t *testing.T) {
	emptyUnitID  := make([]byte, unitIDLength)
	randomUnitID := test.RandomBytes(unitIDLength)
	randomHash   := test.RandomBytes(unitIDLength - typeIDLength)

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
				id:        emptyUnitID,
				hashValue: []byte{},
			},
			want: emptyUnitID,
		}, {
			name: "random",
			args: args{
				id:        randomUnitID,
				hashValue: randomHash,
			},
			want: append(copyAndAppend(randomUnitID[:4], randomHash[4:]...), randomUnitID[typeIDIndex:]...),
		},{
			name: "id empty",
			args: args{
				id:        emptyUnitID,
				hashValue: randomHash,
			},
			want: append(copyAndAppend(emptyUnitID[:4], randomHash[4:]...), emptyUnitID[typeIDIndex:]...),
		},{
			name: "hash empty",
			args: args{
				id:        randomUnitID,
				hashValue: []byte{},
			},
			want: append(copyAndAppend(randomUnitID[:4], emptyUnitID[4:typeIDIndex]...), randomUnitID[typeIDIndex:]...),
		}, {
			name: "hash half empty",
			args: args{
				id:        randomUnitID,
				hashValue: randomHash[:16],
			},
			want: append(append(copyAndAppend(randomUnitID[:4], randomHash[4:16]...), emptyUnitID[16:typeIDIndex]...), randomUnitID[typeIDIndex:]...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SameShardID(tt.args.id, tt.args.hashValue, typeIDLength)
			assert.Equalf(t, tt.want, actual, "sameShardId(%v, %v, %v)", tt.args.id, tt.args.hashValue, typeIDLength)
		})
	}
}

// Makes a copy of the slice before appending
func copyAndAppend(slice []byte, elems ...byte) []byte {
	newSlice := make([]byte, len(slice), len(slice)+len(elems))
	copy(newSlice, slice)

	return append(newSlice, elems...)
}
