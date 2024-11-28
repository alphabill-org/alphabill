package smt

import (
	"crypto"
	"crypto/sha256"
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
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

func (t *TestData) AddToHasher(hasher abhash.Hasher) {
	hasher.Write(t.value)
}

func TestNewSMTWithoutData(t *testing.T) {
	smt, err := New(abhash.New(sha256.New()), 4, []Data{})
	require.NoError(t, err)
	require.NotNil(t, smt)
}

func TestNewSMTWithInvalidKeyLength(t *testing.T) {
	values := []Data{&TestData{value: []byte{0x00, 0xFF}}}
	_, err := New(abhash.New(sha256.New()), 1, values)
	require.ErrorIs(t, err, ErrInvalidKeyLength)
}

func TestNewSMTWithData(t *testing.T) {
	values := []Data{&TestData{value: []byte{0x00, 0xFF}}}
	smt, err := New(abhash.New(sha256.New()), 2, values)
	require.NoError(t, err)
	require.NotNil(t, smt)
	hasher := abhash.New(sha256.New())
	values[0].AddToHasher(hasher)
	valueHash, err := hasher.Sum()
	require.NoError(t, err)
	zeroHash := make([]byte, hasher.Size())
	hasher.Reset()
	for i := 0; i < 8; i++ {
		hasher.Write(zeroHash)
		hasher.Write(valueHash)
		valueHash, err = hasher.Sum()
		require.NoError(t, err)
		hasher.Reset()
	}
	for i := 0; i < 8; i++ {
		hasher.Write(valueHash)
		hasher.Write(zeroHash)
		valueHash, err = hasher.Sum()
		require.NoError(t, err)
		hasher.Reset()
	}
	require.Equal(t, valueHash, smt.root.hash)
}

func TestBuildSMTAndGetAllAuthPaths(t *testing.T) {
	var values []Data
	for i := uint8(0); i < 255; i++ {
		values = append(values, &TestData{[]byte{i}})
	}
	values = append(values, &TestData{[]byte{0xFF}})
	smt, err := New(abhash.New(sha256.New()), 1, values)
	require.NoError(t, err)
	smtRoot := smt.GetRootHash()
	hasher := abhash.New(sha256.New())
	for _, v := range values {
		path, data, err := smt.GetAuthPath(v.Key())
		require.NoError(t, err)
		require.Equal(t, v, data)
		require.NotNil(t, path)
		v.AddToHasher(hasher)
		leaf, err := hasher.Sum()
		require.NoError(t, err)
		hasher.Reset()
		pathRoot, err := CalculatePathRoot(path, leaf, v.Key(), crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, smtRoot, pathRoot)
	}
}

func TestGetAuthPath(t *testing.T) {
	key := []byte{0x00, 0x00, 0x00, 0x00}
	key2 := []byte{0x00, 0x00, 0x00, 0x01}
	values := []Data{&TestData{value: key}, &TestData{value: key2}}
	smt, err := New(abhash.New(sha256.New()), 4, values)
	require.NoError(t, err)

	// key 1
	path, data, err := smt.GetAuthPath(key)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, path)
	require.Equal(t, len(key)*8, len(path))
	require.Equal(t, values[0], data)
	root, err := CalculatePathRoot(path, dataHash(values[0]), key, crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, smt.root.hash, root)

	// key2
	path, data, err = smt.GetAuthPath(key2)
	require.NoError(t, err)
	root, err = CalculatePathRoot(path, dataHash(data), key2, crypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, smt.root.hash, root)
}

func TestPrettyPrint(t *testing.T) {
	key := []byte{0x45, 0x32, 0x45, 0x32}
	key2 := []byte{0x00, 0xA2, 0x45, 0x32}
	values := []Data{&TestData{value: key}, &TestData{value: key2}}
	smt, _ := New(abhash.New(sha256.New()), 4, values)
	require.NotZero(t, len(smt.PrettyPrint()))
}

func TestCalculateRoot_InvalidPathKeyCombination(t *testing.T) {
	_, err := CalculatePathRoot(nil, nil, []byte{0x00}, crypto.SHA256)
	require.NotNil(t, err)
	require.ErrorContains(t, err, "invalid path/key combination")
}

func TestCalculateRoot_InvalidLeafHash(t *testing.T) {
	_, err := CalculatePathRoot([][]byte{
		{0}, {0}, {0}, {0}, {0}, {0}, {0}, {0},
	}, []byte{}, []byte{0}, crypto.SHA256)
	require.ErrorContains(t, err, "invalid leaf hash length")
}

func TestCalculateRoot_KeyIsNil(t *testing.T) {
	_, err := CalculatePathRoot([][]byte{}, nil, nil, crypto.SHA256)
	require.ErrorIs(t, err, ErrInvalidKeyLength)
}

func TestGetAuthPath_DataNotPresent(t *testing.T) {
	key := []byte{0x11, 0x12}
	var values []Data
	smt, err := New(abhash.New(sha256.New()), 2, values)
	require.NoError(t, err)
	path, data, err := smt.GetAuthPath(key)
	require.NoError(t, err)
	require.Nil(t, data)
	require.NotNil(t, path)
}

func dataHash(d Data) []byte {
	hasher := abhash.New(sha256.New())
	d.AddToHasher(hasher)
	h, _ := hasher.Sum()
	return h
}
