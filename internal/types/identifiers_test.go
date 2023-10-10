package types

import (
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/assert"
)

const (
	typePartLength = 1
	unitPartLength = 32
	unitIDLength   = unitPartLength + typePartLength
)

func TestNewUnitID(t *testing.T) {
	emptyUnitID := make([]byte, unitIDLength)
	randomUnitID := test.RandomBytes(unitIDLength)
	randomUnitPart := test.RandomBytes(unitPartLength)
	randomTypePart := test.RandomBytes(typePartLength)
	shardPartLength := 0

	type args struct {
		shardPart []byte
		unitPart  []byte
		typePart  []byte
	}
	tests := []struct {
		name string
		args args
		want UnitID
	}{
		{
			name: "all zeroes",
			args: args{
				shardPart: emptyUnitID,
				unitPart:  []byte{},
				typePart:  []byte{0},
			},
			want: emptyUnitID,
		}, {
			name: "all random",
			args: args{
				shardPart: randomUnitID,
				unitPart:  randomUnitPart,
				typePart:  randomTypePart,
			},
			want: append(copyAndAppend(randomUnitID[:shardPartLength], randomUnitPart[shardPartLength:]...), randomTypePart...),
		}, {
			name: "shardPart nil",
			args: args{
				shardPart: nil,
				unitPart:  randomUnitPart,
				typePart:  randomTypePart,
			},
			want: copyAndAppend(randomUnitPart, randomTypePart...),
		}, {
			name: "shardPart empty",
			args: args{
				shardPart: emptyUnitID,
				unitPart:  randomUnitPart,
				typePart:  randomTypePart,
			},
			want: append(copyAndAppend(emptyUnitID[:shardPartLength], randomUnitPart[shardPartLength:]...), randomTypePart...),
		}, {
			name: "unitPart nil",
			args: args{
				shardPart: randomUnitID,
				unitPart:  nil,
				typePart:  randomTypePart,
			},
			want: append(copyAndAppend(randomUnitID[:shardPartLength], emptyUnitID[shardPartLength:unitPartLength]...), randomTypePart...),
		}, {
			name: "unitPart empty",
			args: args{
				shardPart: randomUnitID,
				unitPart:  []byte{},
				typePart:  randomTypePart,
			},
			want: append(copyAndAppend(randomUnitID[:shardPartLength], emptyUnitID[shardPartLength:unitPartLength]...), randomTypePart...),
		}, {
			name: "unitPart half length",
			args: args{
				shardPart: randomUnitID,
				unitPart:  randomUnitPart[:16],
				typePart:  randomTypePart,
			},
			want: append(append(copyAndAppend(randomUnitID[:shardPartLength], emptyUnitID[16+shardPartLength:unitPartLength]...), randomUnitPart[:16]...), randomTypePart...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := NewUnitID(unitIDLength, tt.args.shardPart, tt.args.unitPart, tt.args.typePart)
			assert.Equalf(t, tt.want, actual, "NewUnitID(%v, %v, %v, %v)", unitIDLength, tt.args.shardPart, tt.args.unitPart, tt.args.typePart)
		})
	}
}

// Makes a copy of the slice before appending
func copyAndAppend(slice []byte, elems ...byte) []byte {
	newSlice := make([]byte, len(slice), len(slice)+len(elems))
	copy(newSlice, slice)

	return append(newSlice, elems...)
}

func TestSystemID_ToSystemID32(t *testing.T) {
	tests := []struct {
		name string
		sid  SystemID
		want SystemID32
	}{
		{
			name: "ID 00000001",
			sid:  SystemID{0, 0, 0, 1},
			want: SystemID32(1),
		},
		{
			name: "ID FF000001",
			sid:  SystemID{0xFF, 0, 0, 1},
			want: SystemID32(0xFF000001),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.sid.ToSystemID32(), "ToSystemID32()")
		})
	}
}

func TestSystemID32_ToSystemID(t *testing.T) {
	tests := []struct {
		name string
		sid  SystemID32
		want SystemID
	}{
		{
			name: "ID 00000001",
			sid:  SystemID32(1),
			want: SystemID{0, 0, 0, 1},
		},
		{
			name: "ID FF000001",
			sid:  SystemID32(0xFF000001),
			want: SystemID{0xFF, 0, 0, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.sid.ToSystemID(), "ToSystemID()")
		})
	}
}
