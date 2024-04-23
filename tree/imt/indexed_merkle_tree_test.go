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

func TestNewIMTWithNilData(t *testing.T) {
	var data []LeafData = nil
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "────┤ empty", imt.PrettyPrint())
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
	path, err := imt.GetMerklePath([]byte{0})
	require.EqualError(t, err, "tree is empty")
	require.Nil(t, path)
	var merklePath []*PathItem = nil
	require.Nil(t, IndexTreeOutput(merklePath, []byte{0}, crypto.SHA256))
}

func TestNewIMTWithEmptyData(t *testing.T) {
	imt, err := New(crypto.SHA256, []LeafData{})
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
	path, err := imt.GetMerklePath([]byte{0})
	require.EqualError(t, err, "tree is empty")
	require.Nil(t, path)
	var merklePath []*PathItem
	require.Nil(t, IndexTreeOutput(merklePath, []byte{0}, crypto.SHA256))
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
	hasher.Write([]byte{tagLeaf})
	hasher.Write(data[0].Key())
	hasher.Write(dataHash)
	require.Equal(t, hasher.Sum(nil), imt.GetRootHash())
	path, err := imt.GetMerklePath(data[0].Key())
	require.NoError(t, err)
	h := IndexTreeOutput(path, data[0].Key(), crypto.SHA256)
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
	require.EqualError(t, err, "data is not sorted by key in strictly ascending order")
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
	require.EqualError(t, err, "data is not sorted by key in strictly ascending order")
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
	require.EqualValues(t, "4FBAE3CCCF457AA68E3F242FBC981989E9ABB9ACEFBE67D509C1617B39BEE03A", fmt.Sprintf("%X", imt.GetRootHash()))
	/* See Yellowpaper appendix C.2.1 Figure 32. Keys of the nodes of an indexed hash tree.
			┌──key: 0a, 08D4FA5D3E55BF5D9B62A4A42DBDDCBE9BFDC121D4A7B9722E53A24AC0C3291D
		┌──key: 09, 71FE84320E781FE3801AF502382064AF0DFAB9E312014286DD519F355C6BDD18
		│	└──key: 09, 6F9842322C9343721ED01CE894C9C35F291FE0237084914D780CF658CF9E7F13
	┌──key: 07, 4FBAE3CCCF457AA68E3F242FBC981989E9ABB9ACEFBE67D509C1617B39BEE03A
	│	│	┌──key: 07, 89982C789FBD76A9905DF9D0140EC8FD62891E6388059627BA65F2C51BD0B176
	│	└──key: 03, 09B830E43D654AFDDF7573657171141D26320FA1629946AE03B1702D556A1E08
	│		│	┌──key: 03, 5377F86D8DD6BA6A8B1F81DD0BE890E1376E27B3127EC780A47A9ACCA3059B71
	│		└──key: 01, F772ACD488C7A878D1EEB22D1603DF00D95370CC4310E75B6A5EBEB191B6C526
	│			└──key: 01, E5A827C8C45F8F4F226286E77F92A5315779467029F90F135FD7F45E14918995
	*/
	treeStr := "\t\t┌──key: 0a, 08D4FA5D3E55BF5D9B62A4A42DBDDCBE9BFDC121D4A7B9722E53A24AC0C3291D\n\t┌──key: 09, 71FE84320E781FE3801AF502382064AF0DFAB9E312014286DD519F355C6BDD18\n\t│\t└──key: 09, 6F9842322C9343721ED01CE894C9C35F291FE0237084914D780CF658CF9E7F13\n┌──key: 07, 4FBAE3CCCF457AA68E3F242FBC981989E9ABB9ACEFBE67D509C1617B39BEE03A\n│\t│\t┌──key: 07, 89982C789FBD76A9905DF9D0140EC8FD62891E6388059627BA65F2C51BD0B176\n│\t└──key: 03, 09B830E43D654AFDDF7573657171141D26320FA1629946AE03B1702D556A1E08\n│\t\t│\t┌──key: 03, 5377F86D8DD6BA6A8B1F81DD0BE890E1376E27B3127EC780A47A9ACCA3059B71\n│\t\t└──key: 01, F772ACD488C7A878D1EEB22D1603DF00D95370CC4310E75B6A5EBEB191B6C526\n│\t\t\t└──key: 01, E5A827C8C45F8F4F226286E77F92A5315779467029F90F135FD7F45E14918995\n"
	require.Equal(t, treeStr, imt.PrettyPrint())
	// check tree node key values
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d.Key(), crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
		// verify data hash
		hasher := crypto.SHA256.New()
		d.AddToHasher(hasher)
		require.EqualValues(t, hasher.Sum(nil), path[0].Hash)
	}
	// test non-inclusion
	idx := []byte{8}
	path, err := imt.GetMerklePath(idx)
	require.NoError(t, err)
	require.NotEqualValues(t, idx, path[0].Key)
	// path still evaluates to root hash
	h := IndexTreeOutput(path, idx, crypto.SHA256)
	require.EqualValues(t, h, imt.GetRootHash())
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
	require.EqualValues(t, "F86304BB5987E61F341A4CC8B396C489321ABA0C601A18B951EB0C1CCB7F9800", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	// check the hash chain of all key nodes
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d.Key(), crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
		// verify data hash
		hasher := crypto.SHA256.New()
		d.AddToHasher(hasher)
		require.EqualValues(t, hasher.Sum(nil), path[0].Hash)
	}
	// non-inclusion
	leaf := TestData{
		key:  util.Uint32ToBytes(uint32(9)),
		data: 9,
	}
	path, err := imt.GetMerklePath(leaf.Key())
	require.NoError(t, err)
	h := IndexTreeOutput(path, leaf.Key(), crypto.SHA256)
	require.EqualValues(t, h, imt.GetRootHash())
	// however, it is not from index 9
	require.NotEqualValues(t, leaf.key, path[0].Key)
	hasher := crypto.SHA256.New()
	leaf.AddToHasher(hasher)
	require.NotEqualValues(t, hasher.Sum(nil), path[0].Hash)
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
	require.EqualValues(t, "3D0A2AC54AAA9EC9E2D2A89BC271E36DCAF84087D13D85E5919BA2C88352830C", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	// check the hash chain of all key nodes
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Key())
		require.NoError(t, err)
		h := IndexTreeOutput(path, d.Key(), crypto.SHA256)
		require.EqualValues(t, h, imt.GetRootHash())
	}
	// non-inclusion
	leaf := TestData{
		key:  util.Uint32ToBytes(uint32(9)),
		data: byte(9),
	}
	path, err := imt.GetMerklePath(leaf.Key())
	require.NoError(t, err)

	h := IndexTreeOutput(path, leaf.Key(), crypto.SHA256)
	require.EqualValues(t, h, imt.GetRootHash())
	// however, it is not from index 9
	require.NotEqualValues(t, leaf.key, path[0].Key)
	hasher := crypto.SHA256.New()
	leaf.AddToHasher(hasher)
	require.NotEqualValues(t, hasher.Sum(nil), path[0].Hash)
}
