package types

import (
	"bytes"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
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

func Test_BytesToSystemID(t *testing.T) {
	t.Run("invalid input", func(t *testing.T) {
		id, err := BytesToSystemID(nil)
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 0 bytes`)

		id, err = BytesToSystemID([]byte{})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 0 bytes`)

		id, err = BytesToSystemID([]byte{1})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 1 bytes`)

		id, err = BytesToSystemID([]byte{2, 1})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 2 bytes`)

		id, err = BytesToSystemID([]byte{3, 2, 1})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 3 bytes`)

		id, err = BytesToSystemID([]byte{5, 4, 3, 2, 1})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 5 bytes`)

		id, err = BytesToSystemID([]byte{8, 7, 6, 5, 4, 3, 2, 1})
		require.Zero(t, id)
		require.EqualError(t, err, `partition ID length must be 4 bytes, got 8 bytes`)
	})

	t.Run("valid input", func(t *testing.T) {
		// testing that we get expected integer from bytes (so basically endianess check?)
		id, err := BytesToSystemID([]byte{0, 0, 0, 0})
		require.NoError(t, err)
		require.EqualValues(t, 0, id)

		id, err = BytesToSystemID([]byte{0, 0, 0, 1})
		require.NoError(t, err)
		require.EqualValues(t, 1, id)

		id, err = BytesToSystemID([]byte{0, 0, 1, 0})
		require.NoError(t, err)
		require.EqualValues(t, 0x0100, id)

		id, err = BytesToSystemID([]byte{0, 1, 0, 0})
		require.NoError(t, err)
		require.EqualValues(t, 0x010000, id)

		id, err = BytesToSystemID([]byte{1, 0, 0, 0})
		require.NoError(t, err)
		require.EqualValues(t, 0x01000000, id)
	})
}

func Test_SystemID_conversion(t *testing.T) {
	t.Run("converting bytes to ID and back", func(t *testing.T) {
		var cases = [][]byte{{0, 0, 0, 0}, {0, 0, 0, 1}, {0, 0, 2, 0}, {0, 3, 0, 0}, {4, 0, 0, 0}, {1, 2, 3, 4}, {0x10, 0xA0, 0xB0, 0xC0}}
		for _, tc := range cases {
			id, err := BytesToSystemID(tc)
			if err != nil {
				t.Errorf("converting bytes %X to ID: %v", tc, err)
				continue
			}
			if b := id.Bytes(); !bytes.Equal(b, tc) {
				t.Errorf("expected ID %s as bytes %X, got %X", id, tc, b)
			}
		}
	})

	t.Run("converting ID to bytes and back", func(t *testing.T) {
		var cases = []SystemID{0, 0x01, 0x0200, 0x030000, 0x04000000, 0xFF, 0xFF12, 0xFEDCBA98}
		for _, tc := range cases {
			b := tc.Bytes()
			id, err := BytesToSystemID(b)
			if err != nil {
				t.Errorf("converting %s (bytes %X) back to ID: %v", tc, b, err)
				continue
			}
			if id != tc {
				t.Errorf("expected %s got %s from bytes %X", tc, id, b)
			}
		}
	})
}

func Test_SystemID_String(t *testing.T) {
	var id SystemID // zero value
	require.Equal(t, "00000000", id.String())

	id = 0x00000001
	require.Equal(t, "00000001", id.String())

	id = 0xFF000010
	require.Equal(t, "FF000010", id.String())

	id = 0xFE01209A
	require.Equal(t, "FE01209A", id.String())
}
