package tree

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

	owner, data, err := tr.Get(uint256.NewInt(0))
	require.Error(t, err)
	require.Nil(t, owner, data)

	exists, err := tr.Exists(uint256.NewInt(9))
	require.NoError(t, err)
	require.False(t, exists)

	require.Error(t, tr.SetData(uint256.NewInt(0), TestData(2)))
	require.Error(t, tr.SetOwner(uint256.NewInt(0), Predicate{1, 2, 3}))
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

	err := tr.Set(uint256.NewInt(0), Predicate{1, 2, 3}, TestData(100))
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

	err := tr.Set(id, owner, data)
	require.NoError(t, err)
	err = tr.Set(uint256.NewInt(1), Predicate{1, 2, 3, 4}, TestData(100))
	require.NoError(t, err)

	require.NotNil(t, tr.GetRootHash())

	genSum := tr.GetSummaryValue()
	sum, ok := genSum.(TestSummaryValue)
	require.True(t, ok, "should be type of TestSummaryValue")
	require.Equal(t, TestSummaryValue(200), sum)

	getOwner, getData, err := tr.Get(id)
	require.NoError(t, err)
	require.Equal(t, owner, getOwner)
	require.Equal(t, data, getData)

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

	err := tr.Set(id, owner1, TestData(1))
	require.NoError(t, err)

	err = tr.SetOwner(id, owner2)
	require.NoError(t, err)

	actualOwner, _, err := tr.Get(id)
	require.NoError(t, err)

	require.Equal(t, owner2, actualOwner)
}

func TestSetData(t *testing.T) {
	tr, _ := New(crypto.SHA256)

	id := uint256.NewInt(0)
	data1 := TestData(1)
	data2 := TestData(2)

	err := tr.Set(id, Predicate{1, 2, 3}, data1)
	require.NoError(t, err)
	sum1 := tr.GetSummaryValue()
	require.Equal(t, TestSummaryValue(1), sum1)

	err = tr.SetData(id, data2)
	require.NoError(t, err)

	_, actualData, err := tr.Get(id)
	require.NoError(t, err)
	require.Equal(t, data2, actualData)

	sum2 := tr.GetSummaryValue()
	require.Equal(t, TestSummaryValue(2), sum2)
}

// TODO
//func TestHashing(t *testing.T) {
//	tr, _ := New(crypto.SHA256)
//
//	id := uint256.NewInt(4)
//	data := TestData(5)
//	owner := Predicate{1, 2, 3}
//	require.NoError(t, tr.Set(id, owner, data))
//
//	rootHash := tr.GetRootHash()
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
