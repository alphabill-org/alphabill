package mt

import (
	"crypto"
	"encoding/hex"
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
	var data []Data = nil
	mt, err := New(crypto.SHA256, data)
	require.Nil(t, mt)
	require.ErrorIs(t, err, ErrNilData)
}

func TestNewMTWithEmptyData(t *testing.T) {
	smt, err := New(crypto.SHA256, []Data{})
	require.NoError(t, err)
	require.NotNil(t, smt)
	require.Equal(t, make([]byte, 32), smt.GetRootHash())
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

func TestSingleNodeTreeMerklePath(t *testing.T) {
	data := []Data{&TestData{hash: make([]byte, 32)}}
	mt, _ := New(crypto.SHA256, data)
	path, err := mt.GetMerklePath(0)
	require.NoError(t, err)
	require.Nil(t, path)
}

func TestMerklePath(t *testing.T) {
	tests := []struct {
		name            string
		dataLength      int
		dataIdxToVerify int
		path            []*PathItem
		wantErr         error
	}{
		{
			name:            "verify leftmost node merkle path",
			dataLength:      8,
			dataIdxToVerify: 0,
			path: []*PathItem{
				{DirectionLeft: true, Hash: decodeHex("0100000000000000000000000000000000000000000000000000000000000000")},
				{DirectionLeft: true, Hash: decodeHex("0094579CFC7B716038D416A311465309BEA202BAA922B224A7B08F01599642FB")},
				{DirectionLeft: true, Hash: decodeHex("633B26EE8A5D96D49A4861E9A5720492F0DB5B6AF305C0B5CFCC6A7EC9B676D4")},
			},
		},
		{
			name:            "verify rightmost node merkle path",
			dataLength:      8,
			dataIdxToVerify: 7,
			path: []*PathItem{
				{DirectionLeft: false, Hash: decodeHex("0600000000000000000000000000000000000000000000000000000000000000")},
				{DirectionLeft: false, Hash: decodeHex("BD50456D5AD175AE99A1612A53CA229124B65D3EAABD9FF9C7AB979A385CF6B3")},
				{DirectionLeft: false, Hash: decodeHex("BA94FFE7EDABF26EF12736F8EB5CE74D15BEDB6AF61444AE2906E926B1A95084")},
			},
		},
		{
			name:            "verify middle node merkle path",
			dataLength:      8,
			dataIdxToVerify: 4,
			path: []*PathItem{
				{DirectionLeft: true, Hash: decodeHex("0500000000000000000000000000000000000000000000000000000000000000")},
				{DirectionLeft: true, Hash: decodeHex("FA670379E5C2212ED93FF09769622F81F98A91E1EC8FB114D607DD25220B9088")},
				{DirectionLeft: false, Hash: decodeHex("BA94FFE7EDABF26EF12736F8EB5CE74D15BEDB6AF61444AE2906E926B1A95084")},
			},
		},
		{
			name:            "verify two node merkle path",
			dataLength:      2,
			dataIdxToVerify: 0,
			path: []*PathItem{
				{DirectionLeft: true, Hash: decodeHex("0100000000000000000000000000000000000000000000000000000000000000")},
			},
		},
		{
			name:            "verify data index out of lower bound",
			dataLength:      8,
			dataIdxToVerify: -1,
			wantErr:         ErrIndexOutOfBounds,
		},
		{
			name:            "verify data index out of upper bound",
			dataLength:      8,
			dataIdxToVerify: 8,
			wantErr:         ErrIndexOutOfBounds,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data = make([]Data, tt.dataLength)
			for i := 0; i < len(data); i++ {
				data[i] = &TestData{hash: makeData(byte(i))}
			}
			mt, _ := New(crypto.SHA256, data)
			merklePath, err := mt.GetMerklePath(tt.dataIdxToVerify)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, merklePath)
				require.Equal(t, len(tt.path), len(merklePath))
				for i := 0; i < len(tt.path); i++ {
					require.EqualValues(t, tt.path[i].DirectionLeft, merklePath[i].DirectionLeft)
					require.EqualValues(t, tt.path[i].Hash, merklePath[i].Hash)
				}
			}
		})
	}
}

func TestMerklePathEval(t *testing.T) {
	tests := []struct {
		name            string
		dataLength      int
		dataIdxToVerify int
	}{
		{
			name:            "verify leftmost node",
			dataLength:      8,
			dataIdxToVerify: 0,
		},
		{
			name:            "verify rightmost node",
			dataLength:      8,
			dataIdxToVerify: 7,
		},
		{
			name:            "verify middle node",
			dataLength:      8,
			dataIdxToVerify: 4,
		},
		{
			name:            "verify leftmost node (odd tree height)",
			dataLength:      4,
			dataIdxToVerify: 0,
		},
		{
			name:            "verify rightmost node (odd tree height)",
			dataLength:      4,
			dataIdxToVerify: 3,
		},
		{
			name:            "verify single node tree",
			dataLength:      1,
			dataIdxToVerify: 0,
		},
		{
			name:            "verify two node tree",
			dataLength:      2,
			dataIdxToVerify: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data = make([]Data, tt.dataLength)
			for i := 0; i < len(data); i++ {
				data[i] = &TestData{hash: makeData(byte(i))}
			}
			mt, _ := New(crypto.SHA256, data)
			merklePath, err := mt.GetMerklePath(tt.dataIdxToVerify)
			require.NoError(t, err)
			rootHash := EvalMerklePath(merklePath, data[tt.dataIdxToVerify], crypto.SHA256)
			require.Equal(t, mt.GetRootHash(), rootHash)
		})
	}
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
		{
			name: "input positive 1337",
			m:    1337,
			n:    1024,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := hibit(tt.m)
			require.Equal(t, tt.n, n)
		})
	}
}

func TestHibitFunction_NegativeInput(t *testing.T) {
	require.PanicsWithValue(t, "hibit function input cannot be negative (merkle tree input data length cannot be zero)", func() {
		hibit(-1)
	})
}

func TestHibitFunction_MaxIntDoesNotOverflow(t *testing.T) {
	n := hibit(math.MaxInt)
	require.True(t, n > 1)
}

func makeData(firstByte byte) []byte {
	data := make([]byte, 32)
	data[0] = firstByte
	return data
}

func decodeHex(s string) []byte {
	decode, _ := hex.DecodeString(s)
	return decode
}
