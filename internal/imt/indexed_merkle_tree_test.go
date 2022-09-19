package imt

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestIMTWithNilInput_ReturnsError(t *testing.T) {
	mt, err := New(nil, crypto.SHA256)
	require.Nil(t, mt)
	require.ErrorIs(t, err, ErrNilData)
}

func TestIMTWithEmptyInput_RootHashIsZeroHash(t *testing.T) {
	imt, err := New([]*Data{}, crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.Equal(t, make([]byte, 32), imt.GetRootHash())
}

func TestIMTWithSingleNode(t *testing.T) {
	hasher := crypto.SHA256.New()
	unitId := uint64(1)
	unitIdBytes := util.Uint64ToBytes(unitId)

	data := []*Data{{Val: unitIdBytes, Hash: unitIdBytes /*LeafHash: unitIdBytes*/}}
	imt, err := New(data, crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, imt)
	require.NotNil(t, imt.GetRootHash())

	hasher.Reset()
	hasher.Write([]byte{1})
	hasher.Write(data[0].Val)
	hasher.Write(data[0].Hash)
	//hasher.Write(data[0].LeafHash)
	expectedRootHash := hasher.Sum(nil)
	require.Equal(t, expectedRootHash, imt.GetRootHash())
}

func TestIMTWithOddNumberOfLeaves(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	var data []*Data
	data = append(data, makeData(1))
	data = append(data, makeData(3))
	data = append(data, makeData(7))
	data = append(data, makeData(9))
	data = append(data, makeData(10))

	// tree visualized
	//             ┌── 10=F8A7A8B6BB47CAF263FD35BA839EAF07E0BF2436D20AF3FD81230E72C8820921
	//        ┌── 9=3D9B4DC05B9C58BA6300D470255D4A6591D3400F2456A7031C60351D534EE6ED
	//        │   └── 9=8B0BBDDC57F3C9748D896C3C7D799A76E93654E450EE301B88C43D705FB0D7F4
	//    ┌── 7=274EE5D3CFEBBDEA53A378675037F1F9EFDDA73DA7BAC95AE69BA9B1411A75C2
	//    │   └── 7=5100C834F488C81BB25FD4A10047A3C3F6C11EA28603C8F82704B275EB80A4F2
	//┌── 3=150174754AA19432198CCC89D50D80F86E96C02816906914786DDA516657547A
	//│   │   ┌── 3=C770D4504A1E34A8AFE428B01B71045D2BAA9EBFACFF3CC560C3585C7A30CF12
	//│   └── 1=80DF7BA4A496FF0710360A6F32CD5A7F76FF0CC762C944782FAD41B712174EF2
	//│       └── 1=0777A62CF9F686541E8D38C65C278DA7A09A67FB52C1297EF7A8FBFEA9E34F5C

	imt, err := New(data, hashAlgorithm)
	fmt.Println(imt.PrettyPrint())
	require.NoError(t, err)
	require.NotNil(t, imt)

	// verify path from every leaf leads to root
	for _, leaf := range data {
		path, _ := imt.GetMerklePath(leaf.Val)
		root := EvalMerklePath(path, leaf, hashAlgorithm)
		require.Equal(t, "150174754AA19432198CCC89D50D80F86E96C02816906914786DDA516657547A", fmt.Sprintf("%X", root),
			"failed to eval path for leaf %X", leaf.Val)
	}
}

func TestIMTWithEvenNumberOfLeaves(t *testing.T) {
	var data []*Data
	data = append(data, makeData(1))
	data = append(data, makeData(3))
	data = append(data, makeData(7))
	data = append(data, makeData(9))
	data = append(data, makeData(11))
	data = append(data, makeData(13))

	// tree visualized
	//             ┌── 13=0F731619DB21F1B913C7EC5D3F4879F40AC18432CB472F2B3ADC1BB3B56AB6D7
	//        ┌── 11=5A7FD64FECB36988C7E0A98D8C995F8AF51143ED8FA171A27D4333F44DAE9E52
	//        │   └── 11=7ECA2DDD8AA1ACB6A8FC9DDD1D6A0A07BD7F98D6A9A494BB7099B89B6BF2250C
	//    ┌── 9=98236D4C72816D9A2E343182A1912D94D64DBBACC02B87C9027F4EFDFD35DB61
	//    │   └── 9=8B0BBDDC57F3C9748D896C3C7D799A76E93654E450EE301B88C43D705FB0D7F4
	//┌── 7=96AEB4BBB770AADC186BF634D7AE3ED0F6100BE612F3662D27019232CC9BED4F
	//│   │       ┌── 7=5100C834F488C81BB25FD4A10047A3C3F6C11EA28603C8F82704B275EB80A4F2
	//│   │   ┌── 3=B6D1C4FE82A10CC1B2A162CCC426844D62B8BECE50E5D39C601DD7B04EB5DEDA
	//│   │   │   └── 3=C770D4504A1E34A8AFE428B01B71045D2BAA9EBFACFF3CC560C3585C7A30CF12
	//│   └── 1=F0398B5F065EDF7DF507DD806097BD4012CD403C72636BD2FD2B72A6401E6BFA
	//│       └── 1=0777A62CF9F686541E8D38C65C278DA7A09A67FB52C1297EF7A8FBFEA9E34F5C
	hashAlgorithm := crypto.SHA256
	imt, err := New(data, hashAlgorithm)
	require.NoError(t, err)
	require.NotNil(t, imt)

	// verify path from every leaf leads to root
	for _, leaf := range data {
		path, _ := imt.GetMerklePath(leaf.Val)
		root := EvalMerklePath(path, leaf, hashAlgorithm)
		require.Equal(t, "96AEB4BBB770AADC186BF634D7AE3ED0F6100BE612F3662D27019232CC9BED4F", fmt.Sprintf("%X", root),
			"failed to eval path for leaf %X", leaf.Val)
	}
}

func TestName(t *testing.T) {
	hasher := crypto.SHA256.New()
	hasher.Write([]byte{1})
	sum := hasher.Sum(nil)
	fmt.Println(fmt.Sprintf("%X", sum))

	hasher.Reset()
	sum = hasher.Sum([]byte{1})
	fmt.Println(fmt.Sprintf("%X", sum))
}

func makeData(num uint64) *Data {
	val := uint256Bytes(num)
	return &Data{Val: val, Hash: hash.Sum256(val)}
}

func uint256Bytes(num uint64) []byte {
	bytes32 := uint256.NewInt(num).Bytes32()
	return bytes32[:]
}
