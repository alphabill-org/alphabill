package imt

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

type TestData struct {
	hash []byte
}

func (t TestData) Hash(hash crypto.Hash) []byte {
	return t.hash
}

func TestNewIMTWithNilData(t *testing.T) {
	var data []Pair = nil
	imt := New(crypto.SHA256, data)
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
}

func TestNewIMTWithEmptyData(t *testing.T) {
	imt := New(crypto.SHA256, []Pair{})
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
}

func TestNewIMTWithSingleNode(t *testing.T) {
	data := []Pair{
		{
			Index: []byte{0, 0, 0, 0},
			Data:  &TestData{hash: make([]byte, 32)},
		},
	}
	mt := New(crypto.SHA256, data)
	require.NotNil(t, mt)
	require.NotNil(t, mt.GetRootHash())
	require.Equal(t, data[0].Hash(crypto.SHA256, Leaf), mt.GetRootHash())
}

func TestNewIMTYellowpaperExample(t *testing.T) {
	var data = []Pair{
		{
			Index: util.Uint32ToBytes(uint32(1)),
			Data:  &TestData{hash: makeData(byte(1))},
		},
		{
			Index: util.Uint32ToBytes(uint32(3)),
			Data:  &TestData{hash: makeData(byte(3))},
		},
		{
			Index: util.Uint32ToBytes(uint32(7)),
			Data:  &TestData{hash: makeData(byte(7))},
		},
		{
			Index: util.Uint32ToBytes(uint32(9)),
			Data:  &TestData{hash: makeData(byte(9))},
		},
		{
			Index: util.Uint32ToBytes(uint32(10)),
			Data:  &TestData{hash: makeData(byte(10))},
		},
	}
	imt := New(crypto.SHA256, data)
	require.NotNil(t, imt)
	require.EqualValues(t, "80F2E3A2A379B86535F3D876CA70D648A256F5D67EADFFC76D0AB6F099AF7664", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	index := util.Uint32ToBytes(uint32(10))
	path, err := imt.GetMerklePath(index)
	require.NoError(t, err)
	require.Len(t, path, 3)
	hash := IndexTreeOutput(path, index, crypto.SHA256)
	require.EqualValues(t, hash, imt.GetRootHash())
	for _, d := range data {
		path, err = imt.GetMerklePath(d.Index)
		require.NoError(t, err)
		hash = EvalMerklePath(path, d, crypto.SHA256)
		require.EqualValues(t, hash, imt.GetRootHash())
	}
	// non-inclusion
	item := Pair{
		Index: util.Uint32ToBytes(uint32(5)),
		Data:  &TestData{hash: makeData(byte(5))},
	}
	path, err = imt.GetMerklePath(item.Index)
	require.NoError(t, err)

	hash = EvalMerklePath(path, item, crypto.SHA256)
	require.NotEqualValues(t, hash, imt.GetRootHash())
}

func TestNewIMTWithOddNumberOfLeaves(t *testing.T) {
	var data = make([]Pair, 5)
	for i := 0; i < len(data); i++ {
		data[i] = Pair{
			Index: util.Uint32ToBytes(uint32(i)),
			Data:  &TestData{hash: makeData(byte(i))},
		}
	}
	imt := New(crypto.SHA256, data)
	require.NotNil(t, imt)
	require.EqualValues(t, "2C811B7D3DD4E7BBD0A9E0C98FFC08495D5C871A6B3B6AD331543889206E1771", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
}

func TestNewIMTWithEvenNumberOfLeaves(t *testing.T) {
	var data = make([]Pair, 8)
	for i := 0; i < len(data); i++ {
		data[i] = Pair{
			Index: util.Uint32ToBytes(uint32(i)),
			Data:  &TestData{hash: makeData(byte(i))},
		}
	}
	imt := New(crypto.SHA256, data)
	require.NotNil(t, imt)
	require.EqualValues(t, "E7A8501605679E35DBF5642EB8372623CD8258FB541680891A77DAF82236B4CE", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
}

func makeData(firstByte byte) []byte {
	data := make([]byte, 32)
	data[0] = firstByte
	return data
}
