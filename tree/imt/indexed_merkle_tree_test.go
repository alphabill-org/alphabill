package imt

import (
	"crypto"
	"fmt"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

type TestData struct {
	key  []byte
	data byte
}

func (t TestData) AddToHasher(hasher hash.Hash) {
	hasher.Write([]byte{t.data})
}

func (t TestData) Key() []byte {
	return t.key
}

func TestIMTNilCases(t *testing.T) {
	imt := &IMT{}
	require.Nil(t, imt.GetRootHash())
	require.EqualValues(t, "────┤ empty", imt.PrettyPrint())
	index := []byte{0}
	path, err := imt.GetMerklePath(index)
	require.EqualError(t, err, "tree empty")
	require.Nil(t, path)
	var pairs []pair
	hashAlgo := crypto.SHA256
	require.EqualValues(t, &node{hash: make([]byte, hashAlgo.Size())}, createMerkleTree(pairs, hashAlgo.New()))
}

func TestNewIMTWithNilData(t *testing.T) {
	var data []LeafData = nil
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
}

func TestNewIMTWithEmptyData(t *testing.T) {
	imt, err := New(crypto.SHA256, []LeafData{})
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
}

func TestNewIMTWithSingleNode(t *testing.T) {
	data := []LeafData{
		&TestData{
			key:  []byte{0, 0, 0, 0},
			data: 1,
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.NotNil(t, imt.GetRootHash())
	hasher := crypto.SHA256.New()
	data[0].AddToHasher(hasher)
	dataHash := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write([]byte{Leaf})
	hasher.Write(data[0].Key())
	hasher.Write(dataHash)
	require.Equal(t, hasher.Sum(nil), imt.GetRootHash())
	path, err := imt.GetMerklePath(data[0].Key())
	h := IndexTreeOutput(path, data[0], crypto.SHA256)
	require.Equal(t, h, imt.GetRootHash())
}

func TestNewIMTUnsortedInput(t *testing.T) {
	var data = []LeafData{
		&TestData{
			key:  util.Uint32ToBytes(uint32(3)),
			data: 3,
		},
		&TestData{
			key:  util.Uint32ToBytes(uint32(1)),
			data: 1,
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.EqualError(t, err, "data not sorted by key in not strictly ascending order")
	require.Nil(t, imt)
}

func TestNewIMTEqualIndexValues(t *testing.T) {
	var data = []LeafData{
		&TestData{
			key:  util.Uint32ToBytes(uint32(1)),
			data: 1,
		},
		&TestData{
			key:  util.Uint32ToBytes(uint32(3)),
			data: 3,
		},
		&TestData{
			key:  util.Uint32ToBytes(uint32(3)),
			data: 3,
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.EqualError(t, err, "data not sorted by key in not strictly ascending order")
	require.Nil(t, imt)
}

func TestNewIMTYellowpaperExample(t *testing.T) {
	var data = []LeafData{
		&TestData{
			key:  []byte{1},
			data: 1,
		},
		&TestData{
			key:  []byte{3},
			data: 3,
		},
		&TestData{
			key:  []byte{7},
			data: 7,
		},
		&TestData{
			key:  []byte{9},
			data: 9,
		},
		&TestData{
			key:  []byte{10},
			data: 10,
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "F0412FC1846E76712137A6D3D3599D0CDEF96BFDE6C4FE7B4C32B52C97B4E311", fmt.Sprintf("%X", imt.GetRootHash()))
	/* See Yellowpaper appendix C.2.1 Figure 33. Identifiers of the nodes of an indexed hash tree
			┌──k: 0a, D5D0B54B518B23C26E9D1C3806FB3A518D0552E44792C6206953C6FABC3E0F93
		┌──k: 09, FBF7CB2DBB870775BC076A92DDD57192E3E3EA3F1FB4DA10ADA1AE28C37B36B0
		│	└──k: 09, CB6B048138A6A4DB1E05E44D55B10975ED8A1752ECE4C766E8336865E523167A
	┌──k: 07, F0412FC1846E76712137A6D3D3599D0CDEF96BFDE6C4FE7B4C32B52C97B4E311
	│	│	┌──k: 07, 77C45ABBB9CB681B371EA253D2E5D4885C3009EE7AFB52AED7D703CED7E1EB8F
	│	└──k: 03, 345CBE8C4856690E162E6774EA6099726E719982DDF78B91814EA104C6FED947
	│		│	┌──k: 03, 1FFF250A0A149718C8B511596CE02AE4FD9EBD330A0BB8428D23EDF13BEFECBC
	│		└──k: 01, CF947354B7ECB39FA99406334DA3D884DFF5FF63D67AB44591CB2930F5BA262E
	│			└──k: 01, 5BD37F59B8EEBFC0D38DC2584912764392400F377056143EFD0434D80CFCEDE9
	*/
	treeStr := "\t\t┌──k: 0a, D5D0B54B518B23C26E9D1C3806FB3A518D0552E44792C6206953C6FABC3E0F93\n\t┌──k: 09, FBF7CB2DBB870775BC076A92DDD57192E3E3EA3F1FB4DA10ADA1AE28C37B36B0\n\t│\t└──k: 09, CB6B048138A6A4DB1E05E44D55B10975ED8A1752ECE4C766E8336865E523167A\n┌──k: 07, F0412FC1846E76712137A6D3D3599D0CDEF96BFDE6C4FE7B4C32B52C97B4E311\n│\t│\t┌──k: 07, 77C45ABBB9CB681B371EA253D2E5D4885C3009EE7AFB52AED7D703CED7E1EB8F\n│\t└──k: 03, 345CBE8C4856690E162E6774EA6099726E719982DDF78B91814EA104C6FED947\n│\t\t│\t┌──k: 03, 1FFF250A0A149718C8B511596CE02AE4FD9EBD330A0BB8428D23EDF13BEFECBC\n│\t\t└──k: 01, CF947354B7ECB39FA99406334DA3D884DFF5FF63D67AB44591CB2930F5BA262E\n│\t\t\t└──k: 01, 5BD37F59B8EEBFC0D38DC2584912764392400F377056143EFD0434D80CFCEDE9\n"
	require.Equal(t, treeStr, imt.PrettyPrint())
	// check tree node key values
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d, crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
	}
	// test non-inclusion
	idx := []byte{8}
	path, err := imt.GetMerklePath(idx)
	require.NoError(t, err)
	item := TestData{
		key:  idx,
		data: 8,
	}
	h := IndexTreeOutput(path, item, crypto.SHA256)
	require.NotEqualValues(t, h, imt.GetRootHash())
}

func TestNewIMTWithOddNumberOfLeaves(t *testing.T) {
	var data = make([]LeafData, 5)
	for i := 0; i < len(data); i++ {
		data[i] = &TestData{
			key:  util.Uint32ToBytes(uint32(i)),
			data: byte(i),
		}
	}
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "F1968402D8A20650D065E9F8731025BAE7C420369F37F7C1B4D6D0BE6F5C9901", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	// check the hash chain of all key nodes
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d, crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
	}
	// non-inclusion
	leaf := TestData{
		key:  util.Uint32ToBytes(uint32(9)),
		data: 9,
	}
	path, err := imt.GetMerklePath(leaf.Key())
	require.NoError(t, err)

	h := IndexTreeOutput(path, leaf, crypto.SHA256)
	require.NotEqualValues(t, h, imt.GetRootHash())
}

func TestNewIMTWithEvenNumberOfLeaves(t *testing.T) {
	var data = make([]LeafData, 8)
	for i := 0; i < len(data); i++ {
		data[i] = &TestData{
			key:  util.Uint32ToBytes(uint32(i)),
			data: byte(i),
		}
	}
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "ACAFD5B38A2A8412B0295D1F6F02695C652FD586C5C6D09F71350EBD0653EEB6", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	// check the hash chain of all key nodes
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d, crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
	}
	// non-inclusion
	leaf := TestData{
		key:  util.Uint32ToBytes(uint32(9)),
		data: byte(9),
	}
	path, err := imt.GetMerklePath(leaf.Key())
	require.NoError(t, err)

	h := IndexTreeOutput(path, leaf, crypto.SHA256)
	require.NotEqualValues(t, h, imt.GetRootHash())
}
