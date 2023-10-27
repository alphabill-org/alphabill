package types

import (
	"testing"

	test "github.com/alphabill-org/alphabill/validator/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestSystemID_Id32(t *testing.T) {
	tests := []struct {
		name    string
		sid     SystemID
		want    SystemID32
		wantErr error
	}{
		{
			name:    "nil",
			sid:     nil,
			wantErr: ErrInvalidSystemIdentifier,
		},
		{
			name:    "ID too big gets truncated FF00000101",
			sid:     SystemID{0xFF, 0, 0, 1, 1},
			wantErr: ErrInvalidSystemIdentifier,
		},
		{
			name:    "ID less than 4 bytes FF00",
			sid:     SystemID{0xFF, 0, 1},
			wantErr: ErrInvalidSystemIdentifier,
		},
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
			got, err := tt.sid.Id32()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.EqualValues(t, 0, got)
				return
			}
			assert.Equalf(t, tt.want, got, "Id32()")
		})
	}
}

func TestSystemID32_String(t *testing.T) {
	var id SystemID32 = 0x00000001
	require.Equal(t, "00000001", id.String())
	id = 0xFF000001
	require.Equal(t, "FF000001", id.String())
	id = 0
	require.Equal(t, "00000000", id.String())
}
