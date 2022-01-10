package state

import (
	"crypto"
	"hash"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type (
	TestData         uint64
	TestSummaryValue uint64
)

func TestEmpty(t *testing.T) {
	tr, err := New(crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, tr)
	require.Nil(t, tr.GetRootHash())
	require.Nil(t, tr.GetSummaryValue())

	unit, err := tr.Get(uint256.NewInt(0))
	require.Error(t, err)
	require.Nil(t, unit)

	exists, err := tr.Exists(uint256.NewInt(9))
	require.NoError(t, err)
	require.False(t, exists)

	require.Error(t, tr.SetData(uint256.NewInt(0), TestData(2), nil))
	require.Error(t, tr.SetOwner(uint256.NewInt(0), Predicate{1, 2, 3}, nil))
}

func TestHashAlgorithms(t *testing.T) {
	tr, err := New(crypto.SHA256)
	require.NoError(t, err)
	require.NotNil(t, tr)

	tr, err = New(crypto.SHA512)
	require.NoError(t, err)
	require.NotNil(t, tr)

	tr, err = New(crypto.MD5)
	require.Errorf(t, err, errStrInvalidHashAlgorithm)
	require.Nil(t, tr)
}

func TestOneItem(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	err := tr.Set(uint256.NewInt(0), Predicate{1, 2, 3}, TestData(100), nil)
	require.NoError(t, err)

	require.NotNil(t, tr.GetRootHash())

	genSum := tr.GetSummaryValue()
	sum, ok := genSum.(TestSummaryValue)
	require.True(t, ok, "should be type of TestSummaryValue")
	require.Equal(t, TestSummaryValue(100), sum)
}

func TestTwoItems(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	id := uint256.NewInt(0)
	owner := Predicate{1, 2, 3}
	data := TestData(100)
	stateHash := []byte("state hash")

	err := tr.Set(id, owner, data, stateHash)
	require.NoError(t, err)
	rootHash1 := tr.GetRootHash()
	err = tr.Set(uint256.NewInt(1), Predicate{1, 2, 3, 4}, TestData(100), []byte("state hash 2"))
	require.NoError(t, err)

	rootHash2 := tr.GetRootHash()
	require.NotNil(t, rootHash2)
	require.NotEqual(t, rootHash1, rootHash2)

	genSum := tr.GetSummaryValue()
	sum, ok := genSum.(TestSummaryValue)
	require.True(t, ok, "should be type of TestSummaryValue")
	require.Equal(t, TestSummaryValue(200), sum)

	unit, err := tr.Get(id)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Bearer)
	require.Equal(t, data, unit.Data)
	require.Equal(t, stateHash, unit.StateHash)

	exists, err := tr.Exists(id)
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = tr.Exists(uint256.NewInt(9))
	require.NoError(t, err)
	require.False(t, exists)
}

func TestSetOwner(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	id := uint256.NewInt(0)
	owner1 := Predicate{1, 2, 3}
	owner2 := Predicate{4, 5, 6}
	stateHash1 := []byte("sh 1")
	stateHash2 := []byte("sh 2")

	err := tr.Set(id, owner1, TestData(1), stateHash1)
	require.NoError(t, err)

	err = tr.SetOwner(id, owner2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.Get(id)
	require.NoError(t, err)

	require.Equal(t, owner2, actualUnit.Bearer)
	require.Equal(t, stateHash2, actualUnit.StateHash)
}

func TestSetData(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	id := uint256.NewInt(0)
	data1 := TestData(1)
	data2 := TestData(2)
	stateHash1 := []byte("sh 1")
	stateHash2 := []byte("sh 2")

	err := tr.Set(id, Predicate{1, 2, 3}, data1, stateHash1)
	require.NoError(t, err)
	sum1 := tr.GetSummaryValue()
	require.Equal(t, TestSummaryValue(1), sum1)

	err = tr.SetData(id, data2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.Get(id)
	require.NoError(t, err)
	require.Equal(t, data2, actualUnit.Data)
	require.Equal(t, stateHash2, actualUnit.StateHash)

	sum2 := tr.GetSummaryValue()
	require.Equal(t, TestSummaryValue(2), sum2)
}

func TestHashing(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	tr, _ := New(hashAlgorithm)

	id := uint256.NewInt(4)
	data := TestData(5)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")
	require.NoError(t, tr.Set(id, owner, data, stateHash))
	actualRootHash := tr.GetRootHash()
	summaryValue := tr.GetSummaryValue()

	var expectedRootHash []byte
	// ID, H(StateHash, H(ID, Bearer, UnitData)), self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
	h := hashAlgorithm.New()
	idBytes := id.Bytes32()

	// Sub hash H(ID, Bearer, UnitData)
	h.Write(idBytes[:])
	h.Write(owner)
	data.AddToHasher(h)
	hashSub1 := h.Sum(nil)

	// Sub hash H(StateHash, subHash1)
	h.Reset()
	h.Write(stateHash)
	h.Write(hashSub1)
	hashSub2 := h.Sum(nil)

	// Main hash H(ID, subHash2, self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
	h.Reset()
	h.Write(idBytes[:])
	h.Write(hashSub2)
	summaryValue.AddToHasher(h)
	// Children are empty
	h.Write(make([]byte, hashAlgorithm.Size()))
	h.Write(make([]byte, hashAlgorithm.Size()))
	expectedRootHash = h.Sum(nil)

	require.Equal(t, expectedRootHash, actualRootHash)
}

func (u TestData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(u)))
}

func (u TestData) Value() SummaryValue {
	return TestSummaryValue(u)
}

func (t TestSummaryValue) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(t)))
}

func (t TestSummaryValue) Concatenate(left, right SummaryValue) SummaryValue {
	var s, l, r uint64
	s = uint64(t)
	lSum, ok := left.(TestSummaryValue)
	if ok {
		l = uint64(lSum)
	}
	rSum, ok := right.(TestSummaryValue)
	if ok {
		r = uint64(rSum)
	}
	return TestSummaryValue(s + l + r)
}
