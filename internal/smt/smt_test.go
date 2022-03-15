package smt

import (
	"crypto/sha256"
	"hash"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestData struct {
	value []byte
}

func (t *TestData) Key(_ int) []byte {
	return t.value
}

func (t *TestData) Value() []byte {
	return t.value
}

func (t *TestData) AddToHasher(hasher hash.Hash) {
	hasher.Write(t.value)
}

func TestNewSMTWithoutData(t *testing.T) {
	smt, err := New(sha256.New(), 4, []Data{})
	require.NoError(t, err)
	require.NotNil(t, smt)
}

func TestNewSMTWithInvalidKeyLength(t *testing.T) {
	values := []Data{&TestData{value: []byte{0x00, 0xFF}}}
	_, err := New(sha256.New(), 1, values)
	require.ErrorIs(t, err, ErrInvalidKeyLength)
}

func TestNewSMTWithData(t *testing.T) {
	values := []Data{&TestData{value: []byte{0x00, 0xFF}}}
	smt, err := New(sha256.New(), 2, values)
	require.NoError(t, err)
	require.NotNil(t, smt)
	hasher := sha256.New()
	values[0].AddToHasher(hasher)
	valueHash := hasher.Sum(nil)
	zeroHash := make([]byte, hasher.Size())
	hasher.Reset()
	for i := 0; i < 7; i++ {
		hasher.Write(zeroHash)
		hasher.Write(valueHash)
		valueHash = hasher.Sum(nil)
		hasher.Reset()
	}
	for i := 0; i < 8; i++ {
		hasher.Write(valueHash)
		hasher.Write(zeroHash)
		valueHash = hasher.Sum(nil)
		hasher.Reset()
	}
	require.Equal(t, valueHash, smt.root.hash)
}

func Test_isBitSet(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  []bool
	}{
		{
			name:  "0x00",
			bytes: []byte{0x00}, //00000000
			want:  []bool{false, false, false, false, false, false, false, false},
		},
		{
			name:  "0xFF",
			bytes: []byte{0xFF}, // 11111111
			want:  []bool{true, true, true, true, true, true, true, true},
		},
		{
			name:  "0x00, 0xFF", // 00000000 11111111
			bytes: []byte{0x00, 0xFF},
			want: []bool{false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true},
		},
		{
			name:  "0x11, 0x12", // 00010001 00010010
			bytes: []byte{0x11, 0x12},
			want: []bool{false, false, false, true, false, false, false, true,
				false, false, false, true, false, false, true, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length := len(tt.bytes) * 8
			for i := 0; i < length; i++ {
				if got := isBitSet(tt.bytes, i); got != tt.want[i] {
					t.Errorf("isBitSet() = %v, want %v", got, tt.want[i])
				}
			}
		})
	}
}

func TestGetAuthPath(t *testing.T) {
	key := []byte{0x00, 0xFF}
	values := []Data{&TestData{value: key}}
	smt, err := New(sha256.New(), 2, values)
	require.NoError(t, err)
	path, data, err := smt.GetAuthPath(key)
	require.NoError(t, err)
	require.Equal(t, values[0], data)
	require.NotNil(t, path)
	require.Equal(t, 16, len(path))
	hasher := sha256.New()
	hasher.Write(key)
	hash := hasher.Sum(nil)
	hasher.Reset()
	for i, pathItem := range path {
		if i == 0 {
			require.Equal(t, hash, pathItem)
			continue
		}
		if isBitSet(key, i) {
			hasher.Write(hash)
			hasher.Write(pathItem)
		} else {
			hasher.Write(pathItem)
			hasher.Write(hash)
		}
		hash = hasher.Sum(nil)
		hasher.Reset()
	}
	require.Equal(t, smt.root.hash, hash)
}

func TestGetAuthPath_DataNotPresent(t *testing.T) {
	key := []byte{0x11, 0x12}
	var values []Data
	smt, err := New(sha256.New(), 2, values)
	require.NoError(t, err)
	path, data, err := smt.GetAuthPath(key)
	require.NoError(t, err)
	require.Nil(t, data)
	require.NotNil(t, path)
}
