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
	require.Nil(t, tr.TotalValue())

	unit, err := tr.get(uint256.NewInt(0))
	require.Error(t, err)
	require.Nil(t, unit)

	exists := tr.exists(uint256.NewInt(9))
	require.False(t, exists)

	require.Error(t, tr.setData(uint256.NewInt(0), TestData(2), nil))
	require.Error(t, tr.setOwner(uint256.NewInt(0), Predicate{1, 2, 3}, nil))
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

	tr.set(uint256.NewInt(0), Predicate{1, 2, 3}, TestData(100), nil)

	require.NotNil(t, tr.GetRootHash())

	genSum := tr.TotalValue()
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

	tr.set(id, owner, data, stateHash)
	rootHash1 := tr.GetRootHash()
	tr.set(uint256.NewInt(1), Predicate{1, 2, 3, 4}, TestData(100), []byte("state hash 2"))

	rootHash2 := tr.GetRootHash()
	require.NotNil(t, rootHash2)
	require.NotEqual(t, rootHash1, rootHash2)

	genSum := tr.TotalValue()
	sum, ok := genSum.(TestSummaryValue)
	require.True(t, ok, "should be type of TestSummaryValue")
	require.Equal(t, TestSummaryValue(200), sum)

	unit, err := tr.get(id)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Bearer)
	require.Equal(t, data, unit.Data)
	require.Equal(t, stateHash, unit.StateHash)

	exists := tr.exists(id)
	require.True(t, exists)

	exists = tr.exists(uint256.NewInt(9))
	require.False(t, exists)
}

func TestSetOwner(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	id := uint256.NewInt(0)
	owner1 := Predicate{1, 2, 3}
	owner2 := Predicate{4, 5, 6}
	stateHash1 := []byte("sh 1")
	stateHash2 := []byte("sh 2")

	tr.set(id, owner1, TestData(1), stateHash1)

	err := tr.setOwner(id, owner2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.get(id)
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

	tr.set(id, Predicate{1, 2, 3}, data1, stateHash1)
	sum1 := tr.TotalValue()
	require.Equal(t, TestSummaryValue(1), sum1)

	err := tr.setData(id, data2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.get(id)
	require.NoError(t, err)
	require.Equal(t, data2, actualUnit.Data)
	require.Equal(t, stateHash2, actualUnit.StateHash)

	sum2 := tr.TotalValue()
	require.Equal(t, TestSummaryValue(2), sum2)
}

func TestHashing(t *testing.T) {
	hashAlgorithm := crypto.SHA256
	tr, _ := New(hashAlgorithm)

	id := uint256.NewInt(4)
	data := TestData(5)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")
	tr.set(id, owner, data, stateHash)
	actualRootHash := tr.GetRootHash()
	summaryValue := tr.TotalValue()

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

func TestAddItem(t *testing.T) {
	tr, _ := New(crypto.SHA256)
	id := uint256.NewInt(4)
	data := TestData(5)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")
	err := tr.AddItem(id, owner, data, stateHash)
	require.NoError(t, err)

	unit, err := tr.get(id)
	requireEqual(t, owner, data, stateHash, unit)

	err = tr.AddItem(id, owner, data, stateHash)
	require.Error(t, err, "Adding existing item must fail")
}

func TestUpdateData(t *testing.T) {
	tr, _ := New(crypto.SHA256)
	id := uint256.NewInt(4)
	data := TestData(5)
	newData := TestData(6)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")

	updateFunc := func(data UnitData) UnitData {
		return newData
	}

	err := tr.UpdateData(id, updateFunc, stateHash)
	require.Errorf(t, err, "updating non existing node must fail")

	err = tr.AddItem(id, owner, data, stateHash)
	require.NoError(t, err)

	err = tr.UpdateData(id, updateFunc, stateHash)
	require.NoError(t, err)
	unit, err := tr.get(id)
	require.NoError(t, err)
	requireEqual(t, owner, newData, stateHash, unit)
}

//func TestDelete(t *testing.T) {
//	tr, _ := New(crypto.SHA256)
//
//	id := uint256.NewInt(4)
//	data := TestData(5)
//	owner := Predicate{1, 2, 3}
//	stateHash := []byte("state hash")
//
//	err := tr.DeleteItem(id)
//	require.Error(t, err, "deleting not existing item must fail")
//
//	err = tr.AddItem(id, owner, data, stateHash)
//	require.NoError(t, err)
//	err = tr.DeleteItem(id)
//	require.NoError(t, err)
//}

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

func requireEqual(t *testing.T, expectedOwner Predicate, expectedData TestData, expectedStateHash []byte, actualUnit *Unit) {
	require.Equal(t, expectedOwner, actualUnit.Bearer)
	require.Equal(t, expectedData, actualUnit.Data)
	require.Equal(t, expectedStateHash, actualUnit.StateHash)
}
