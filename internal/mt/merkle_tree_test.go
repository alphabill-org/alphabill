package mt

import (
	"crypto"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestData struct {
	hash []byte
}

func (t *TestData) Hash(hash crypto.Hash) []byte {
	return t.hash
}

func TestNewMTWithNilData(t *testing.T) {
	mt, err := New(crypto.SHA256, nil)
	require.Nil(t, mt)
	require.ErrorIs(t, err, ErrNilData)
}

func TestNewMTWithEmptyData(t *testing.T) {
	smt, err := New(crypto.SHA256, []Data{})
	require.NoError(t, err)
	require.NotNil(t, smt)
	require.Nil(t, smt.GetRootHash())
}

func TestNewMTWithSingleNode(t *testing.T) {
	data := []Data{&TestData{hash: make([]byte, 32)}}
	mt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, mt)
	require.NotNil(t, mt.GetRootHash())
	require.Equal(t, data[0].Hash(crypto.SHA256), mt.GetRootHash())
}

func TestNewMTWithOddNumberOfLeaves(t *testing.T) {
	var data = make([]Data, 7)
	for i := 0; i < len(data); i++ {
		data[i] = &TestData{hash: makeData(byte(i))}
	}
	mt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, mt)
	require.EqualValues(t, "47A288CB996BFAAA0703D976C338841884F938C06E62A161E4772D6FB68A4A69", fmt.Sprintf("%X", mt.GetRootHash()))
}

func TestNewMTWithEvenNumberOfLeaves(t *testing.T) {
	var data = make([]Data, 8)
	for i := 0; i < len(data); i++ {
		data[i] = &TestData{hash: makeData(byte(i))}
	}
	mt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, mt)
	require.EqualValues(t, "89A0F1577268CC19B0A39C7A69F804FD140640C699585EB635EBB03C06154CCE", fmt.Sprintf("%X", mt.GetRootHash()))
}

func TestHibitFunction_NormalInput(t *testing.T) {
	tests := []struct {
		name string
		m    int
		n    int
	}{
		{
			name: "input zero",
			m:    0,
			n:    0,
		},
		{
			name: "input positive 1",
			m:    1,
			n:    1,
		},
		{
			name: "input positive 25",
			m:    25,
			n:    16,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := hibit(tt.m)
			require.Equal(t, tt.n, n)
		})
	}
}

func TestHibitFunction_IntegerOverflow(t *testing.T) {
	require.Panics(t, func() {
		hibit(math.MaxInt)
	}, "integer overflow")
}

func TestHibitFunction_NegativeInput(t *testing.T) {
	require.Panics(t, func() {
		hibit(math.MaxInt)
	}, "hibit function input cannot be negative (merkle tree input data length cannot be zero)")
}

func makeData(firstByte byte) []byte {
	data := make([]byte, 32)
	data[0] = firstByte
	return data
}
