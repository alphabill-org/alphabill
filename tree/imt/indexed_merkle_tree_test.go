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

func TestIMTNilCases(t *testing.T) {
	imt := &IMT{}
	require.Nil(t, imt.GetRootHash())
	require.EqualValues(t, "────┤ empty", imt.PrettyPrint())
	index := []byte{0}
	path, err := imt.GetMerklePath(index)
	require.EqualError(t, err, "tree empty")
	require.Nil(t, path)
	var data []Pair
	hashAlgo := crypto.SHA256
	require.EqualValues(t, &node{hash: make([]byte, hashAlgo.Size())}, createMerkleTree(data, hashAlgo))
}

func TestNewIMTWithNilData(t *testing.T) {
	var data []Pair = nil
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.Nil(t, imt.GetRootHash())
	require.Equal(t, 0, imt.dataLength)
}

func TestNewIMTWithEmptyData(t *testing.T) {
	imt, err := New(crypto.SHA256, []Pair{})
	require.NoError(t, err)
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
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.NotNil(t, imt.GetRootHash())
	require.Equal(t, data[0].Hash(crypto.SHA256), imt.GetRootHash())
	path, err := imt.GetMerklePath(data[0].Index)
	hash := IndexTreeOutput(path, data[0].Index, data[0].Data, crypto.SHA256)
	require.Equal(t, hash, imt.GetRootHash())
}

func TestNewIMTUnsortedInput(t *testing.T) {
	var data = []Pair{
		{
			Index: util.Uint32ToBytes(uint32(3)),
			Data:  &TestData{hash: makeData(byte(1))},
		},
		{
			Index: util.Uint32ToBytes(uint32(1)),
			Data:  &TestData{hash: makeData(byte(3))},
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.EqualError(t, err, "data not sorted by index in not strictly ascending order")
	require.Nil(t, imt)
}

func TestNewIMTEqualIndexValues(t *testing.T) {
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
			Index: util.Uint32ToBytes(uint32(3)),
			Data:  &TestData{hash: makeData(byte(4))},
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.EqualError(t, err, "data not sorted by index in not strictly ascending order")
	require.Nil(t, imt)
}

func TestNewIMTYellowpaperExample(t *testing.T) {
	var data = []Pair{
		{
			Index: []byte{1},
			Data:  &TestData{hash: makeData(byte(1))},
		},
		{
			Index: []byte{3},
			Data:  &TestData{hash: makeData(byte(3))},
		},
		{
			Index: []byte{7},
			Data:  &TestData{hash: makeData(byte(7))},
		},
		{
			Index: []byte{9},
			Data:  &TestData{hash: makeData(byte(9))},
		},
		{
			Index: []byte{10},
			Data:  &TestData{hash: makeData(byte(10))},
		},
	}
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "29D043A78E460AEE1F6F60B00D7EFE74AF24C226FB539953B0739484ABAEB36C", fmt.Sprintf("%X", imt.GetRootHash()))
	/* See Yellowpaper appendix C.2.1 Figure 33. Identifiers of the nodes of an indexed hash tree
			┌──i: 0a, 343BCD7418B69AD66089CEAF0DC08174D2C3F732CD85931E298FF11F5E7C19D1
		┌──i: 09, 013A8580AB6DB15A3532CEE1C904D9C0C64760B98BC83956AACEE1C7AC6D02EF
		│	└──i: 09, 71713F4B262AB8FC8122F51A634B916CCA2471C17486DB4708ADF60997C8C8F5
	┌──i: 07, 29D043A78E460AEE1F6F60B00D7EFE74AF24C226FB539953B0739484ABAEB36C
	│	│	┌──i: 07, 18BB7F19ED9D49774E5A4DF0495F0C20CC1023D3B946BD136D270A7CE82C9D66
	│	└──i: 03, 9A990CEAAD4D93E3EA4A109F15DB6FC903C5634F35FB19873BAC4DD31BED5CE0
	│		│	┌──i: 03, 87579BD04E8AC8FF9345E97BE9D30692594FFBEF3D663B9622644D7BA337F337
	│		└──i: 01, 5BC2AE158BEEE30DF08902A34C6BC703D0A9AB3485E062B81C47AD5A6C5955AF
	│			└──i: 01, 5FFD74085B510C6100911078A4118BC91D0D50215F364B9036BF7167DA7C412F
	*/
	treeStr := "\t\t┌──i: 0a, 343BCD7418B69AD66089CEAF0DC08174D2C3F732CD85931E298FF11F5E7C19D1\n\t┌──i: 09, 013A8580AB6DB15A3532CEE1C904D9C0C64760B98BC83956AACEE1C7AC6D02EF\n\t│\t└──i: 09, 71713F4B262AB8FC8122F51A634B916CCA2471C17486DB4708ADF60997C8C8F5\n┌──i: 07, 29D043A78E460AEE1F6F60B00D7EFE74AF24C226FB539953B0739484ABAEB36C\n│\t│\t┌──i: 07, 18BB7F19ED9D49774E5A4DF0495F0C20CC1023D3B946BD136D270A7CE82C9D66\n│\t└──i: 03, 9A990CEAAD4D93E3EA4A109F15DB6FC903C5634F35FB19873BAC4DD31BED5CE0\n│\t\t│\t┌──i: 03, 87579BD04E8AC8FF9345E97BE9D30692594FFBEF3D663B9622644D7BA337F337\n│\t\t└──i: 01, 5BC2AE158BEEE30DF08902A34C6BC703D0A9AB3485E062B81C47AD5A6C5955AF\n│\t\t\t└──i: 01, 5FFD74085B510C6100911078A4118BC91D0D50215F364B9036BF7167DA7C412F\n"
	require.Equal(t, treeStr, imt.PrettyPrint())
	// check tree node index values
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Index)
		require.NoError(t, err)
		hash := IndexTreeOutput(path, d.Index, d.Data, crypto.SHA256)
		require.EqualValues(t, hash, imt.GetRootHash())
	}
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Index)
		require.NoError(t, err)
		hash := EvalMerklePath(path, d, crypto.SHA256)
		require.EqualValues(t, hash, imt.GetRootHash())
	}
	// test non-inclusion
	item := Pair{
		Index: util.Uint32ToBytes(uint32(5)),
		Data:  &TestData{hash: makeData(byte(5))},
	}
	path, err := imt.GetMerklePath(item.Index)
	require.NoError(t, err)

	hash := EvalMerklePath(path, item, crypto.SHA256)
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
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
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
	imt, err := New(crypto.SHA256, data)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.EqualValues(t, "E7A8501605679E35DBF5642EB8372623CD8258FB541680891A77DAF82236B4CE", fmt.Sprintf("%X", imt.GetRootHash()))
	require.NotEmpty(t, imt.PrettyPrint())
	for _, d := range data {
		path, err := imt.GetMerklePath(d.Index)
		require.NoError(t, err)
		hash := EvalMerklePath(path, d, crypto.SHA256)
		require.EqualValues(t, hash, imt.GetRootHash())
	}
	// non-inclusion
	item := Pair{
		Index: util.Uint32ToBytes(uint32(9)),
		Data:  &TestData{hash: makeData(byte(9))},
	}
	path, err := imt.GetMerklePath(item.Index)
	require.NoError(t, err)

	hash := EvalMerklePath(path, item, crypto.SHA256)
	require.NotEqualValues(t, hash, imt.GetRootHash())
}

func makeData(firstByte byte) []byte {
	data := make([]byte, 32)
	data[0] = firstByte
	return data
}
