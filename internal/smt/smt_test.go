package smt

import (
	"crypto"
	"crypto/sha256"
	"hash"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestData struct {
	value []byte
}

func (t *TestData) Key() []byte {
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

func TestGetAuthPath(t *testing.T) {
	key := []byte{0x45, 0x32, 0x45, 0x32}
	key2 := []byte{0x00, 0xA2, 0x45, 0x32}
	values := []Data{&TestData{value: key}, &TestData{value: key2}}
	smt, err := New(sha256.New(), 4, values)
	require.NoError(t, err)
	path, data, err := smt.GetAuthPath(key)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, path)
	require.Equal(t, len(key)*8-1, len(path))
	hasher := sha256.New()
	data.AddToHasher(hasher)
	leaf := hasher.Sum(nil)
	smt.GetAuthPath([]byte{0, 0, 0, 1})
	root, err := CalculatePathRoot(path, leaf, key, crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, smt.root.hash, root)
}

func TestPrettyPrint(t *testing.T) {
	key := []byte{0x45, 0x32, 0x45, 0x32}
	key2 := []byte{0x00, 0xA2, 0x45, 0x32}
	values := []Data{&TestData{value: key}, &TestData{value: key2}}
	smt, _ := New(sha256.New(), 4, values)
	require.NotZero(t, len(smt.PrettyPrint()))
}

func TestCalculateRoot_InvalidPathKeyCombination(t *testing.T) {
	_, err := CalculatePathRoot(nil, nil, []byte{}, crypto.SHA256)
	require.NotNil(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid path/key combination"))
}

func TestCalculateRoot_InvalidLeafHash(t *testing.T) {
	_, err := CalculatePathRoot([][]byte{
		{0}, {0}, {0}, {0}, {0}, {0}, {0},
	}, []byte{}, []byte{0}, crypto.SHA256)
	require.True(t, strings.Contains(err.Error(), "invalid leaf hash length"))
}

func TestCalculateRoot_KeyIsNil(t *testing.T) {
	_, err := CalculatePathRoot([][]byte{}, nil, nil, crypto.SHA256)
	require.Error(t, ErrInvalidKeyLength, err)
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
