package omt

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestOMTWithNilInput_ReturnsError(t *testing.T) {
	tree, err := New(nil, crypto.SHA256)
	require.Nil(t, tree)
	require.ErrorIs(t, err, ErrNilData)
}

func TestOMTWithEmptyInput_RootHashIsZeroHash(t *testing.T) {
	tree, err := New([]*Data{}, crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, tree)
	require.Equal(t, make([]byte, 32), tree.GetRootHash())
}

func TestOMTWithSingleNode(t *testing.T) {
	unitId := uint64(1)
	unitIdBytes := util.Uint64ToBytes(unitId)

	data := []*Data{{Val: unitIdBytes, Hash: unitIdBytes}}
	tree, err := New(data, crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, tree)
	require.NotNil(t, tree.GetRootHash())

	expectedRootHash := computeLeafTreeHash(data[0], crypto.SHA256.New())
	require.Equal(t, expectedRootHash, tree.GetRootHash())
}

func TestOMTWithOddNumberOfLeaves(t *testing.T) {
	var data []*Data
	data = append(data, makeData(1))
	data = append(data, makeData(3))
	data = append(data, makeData(7))
	data = append(data, makeData(9))
	data = append(data, makeData(10))

	// tree visualized
	//            ┌── 13=0F731619DB21F1B913C7EC5D3F4879F40AC18432CB472F2B3ADC1BB3B56AB6D7
	//        ┌── 11=2B10576493FB62A47CBDD1CB861EB1F2F16A40DC8BDE3386F814B830916626FA
	//        │   └── 11=7ECA2DDD8AA1ACB6A8FC9DDD1D6A0A07BD7F98D6A9A494BB7099B89B6BF2250C
	//    ┌── 9=B30B365BB543EC0D60BE3BE96AF940B54004B96A6F67EA48EE1D8C3DD6F1834B
	//    │   └── 9=8B0BBDDC57F3C9748D896C3C7D799A76E93654E450EE301B88C43D705FB0D7F4
	//┌── 7=48BDD6E1C79318C2D73203637BEA5B795A0C72A64AE20B5BF6F51D6854AF55E2
	//│   │       ┌── 7=5100C834F488C81BB25FD4A10047A3C3F6C11EA28603C8F82704B275EB80A4F2
	//│   │   ┌── 3=5F3BBEC2F6E9128A5CFB1DDACE68CD1CD7645CB76A5D6E6DA61C16682900656E
	//│   │   │   └── 3=C770D4504A1E34A8AFE428B01B71045D2BAA9EBFACFF3CC560C3585C7A30CF12
	//│   └── 1=2E4D40A18147557430F581747B3AF7F4A0FD5AD2833F99B765770FB9CB0E6E0C
	//│       └── 1=0777A62CF9F686541E8D38C65C278DA7A09A67FB52C1297EF7A8FBFEA9E34F5C

	hashAlgorithm := crypto.SHA256
	tree, err := New(data, hashAlgorithm)
	require.NoError(t, err)
	require.NotNil(t, tree)

	// verify path from every leaf leads to root
	for _, d := range data {
		path, _ := tree.GetMerklePath(d.Val)
		root := EvalMerklePath(path, d.Val, hashAlgorithm)
		require.Equal(t, "9B7E50B44CA70564BD9EC7269A4AB4A7C6E55299E2B3DF0E66D5B37B2381C095", fmt.Sprintf("%X", root),
			"failed to eval path for leaf %X", d.Val)
	}
}

func TestOMTWithEvenNumberOfLeaves(t *testing.T) {
	var data []*Data
	data = append(data, makeData(1))
	data = append(data, makeData(3))
	data = append(data, makeData(7))
	data = append(data, makeData(9))
	data = append(data, makeData(11))
	data = append(data, makeData(13))

	// tree visualized
	//            ┌── 13=0F731619DB21F1B913C7EC5D3F4879F40AC18432CB472F2B3ADC1BB3B56AB6D7
	//        ┌── 11=2B10576493FB62A47CBDD1CB861EB1F2F16A40DC8BDE3386F814B830916626FA
	//        │   └── 11=7ECA2DDD8AA1ACB6A8FC9DDD1D6A0A07BD7F98D6A9A494BB7099B89B6BF2250C
	//    ┌── 9=B30B365BB543EC0D60BE3BE96AF940B54004B96A6F67EA48EE1D8C3DD6F1834B
	//    │   └── 9=8B0BBDDC57F3C9748D896C3C7D799A76E93654E450EE301B88C43D705FB0D7F4
	//┌── 7=48BDD6E1C79318C2D73203637BEA5B795A0C72A64AE20B5BF6F51D6854AF55E2
	//│   │       ┌── 7=5100C834F488C81BB25FD4A10047A3C3F6C11EA28603C8F82704B275EB80A4F2
	//│   │   ┌── 3=5F3BBEC2F6E9128A5CFB1DDACE68CD1CD7645CB76A5D6E6DA61C16682900656E
	//│   │   │   └── 3=C770D4504A1E34A8AFE428B01B71045D2BAA9EBFACFF3CC560C3585C7A30CF12
	//│   └── 1=2E4D40A18147557430F581747B3AF7F4A0FD5AD2833F99B765770FB9CB0E6E0C
	//│       └── 1=0777A62CF9F686541E8D38C65C278DA7A09A67FB52C1297EF7A8FBFEA9E34F5C
	hashAlgorithm := crypto.SHA256
	tree, err := New(data, hashAlgorithm)
	require.NoError(t, err)
	require.NotNil(t, tree)

	// verify path from every leaf leads to root
	for _, d := range data {
		path, _ := tree.GetMerklePath(d.Val)
		root := EvalMerklePath(path, d.Val, hashAlgorithm)
		require.Equal(t, "48BDD6E1C79318C2D73203637BEA5B795A0C72A64AE20B5BF6F51D6854AF55E2", fmt.Sprintf("%X", root),
			"failed to eval path for leaf %X", d.Val)
	}
}

func makeData(num uint64) *Data {
	val := uint256Bytes(num)
	return &Data{Val: val, Hash: hash.Sum256(val)}
}

func uint256Bytes(num uint64) []byte {
	bytes32 := uint256.NewInt(num).Bytes32()
	return bytes32[:]
}
