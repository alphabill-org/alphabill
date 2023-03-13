package memorydb

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

var defaultsDBKeys = []string{"1", "2", "3", "4"}

func initDB(t *testing.T, defaults []string) *MemoryDB {
	t.Helper()
	memDB := New()
	require.NotNil(t, memDB)
	if defaults == nil {
		return memDB
	}
	// init with default values
	for idx, key := range defaults {
		require.NoError(t, memDB.Write([]byte(key), strconv.Itoa(idx)))
	}
	return memDB
}

func TestIterator_Nil(t *testing.T) {
	it := NewIterator(nil, json.Unmarshal)
	require.False(t, it.Valid())
}

func TestIterator_TestEmptyDB(t *testing.T) {
	db := initDB(t, nil)
	it := db.First()
	defer it.Close()
	require.False(t, it.Valid())
	var value string
	require.ErrorContains(t, it.Value(value), "iterator invalid")
	require.Len(t, it.Key(), 0)
}

func TestIterator_TestIterator(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	it := db.First()
	defer it.Close()
	iterations := 0
	for ; it.Valid(); it.Next() {
		require.Equal(t, []byte(defaultsDBKeys[iterations]), it.Key())
		var value string
		require.NoError(t, it.Value(&value))
		require.Equal(t, strconv.Itoa(iterations), value)
		iterations++
	}
	require.Equal(t, len(defaultsDBKeys), iterations)
}

func TestIterator_TestIteratorReverse(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	it := db.Last()
	defer it.Close()
	require.True(t, it.Valid())
	require.Equal(t, []byte("4"), it.Key())
	iterations := 0
	for ; it.Valid(); it.Prev() {
		require.Equal(t, []byte(defaultsDBKeys[len(defaultsDBKeys)-iterations-1]), it.Key())
		var value string
		require.NoError(t, it.Value(&value))
		require.Equal(t, strconv.Itoa(len(defaultsDBKeys)-iterations-1), value)
		iterations++
	}
	require.Equal(t, len(defaultsDBKeys), iterations)
}

func TestIterator_TestIteratorSeek(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	it := db.Find([]byte("3"))
	defer it.Close()
	require.True(t, it.Valid())
	require.Equal(t, []byte("3"), it.Key())
	var value string
	require.NoError(t, it.Value(&value))
	// default value is one less
	require.Equal(t, "2", value)
}

func TestIterator_FindNoMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	// seek past end
	it := db.Find([]byte("waypastend"))
	defer it.Close()
	require.False(t, it.Valid())
}

func TestIterator_FindClosestMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	// seek past end
	it := db.Find([]byte("0"))
	defer it.Close()
	require.True(t, it.Valid())
	require.Equal(t, []byte("1"), it.Key())
}

func TestBoltIterator_DoubleClose(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	// seek past end
	it := db.Find([]byte("0"))
	require.NoError(t, it.Close())
	require.NoError(t, it.Close())
}

func TestBoltIterator_IteratePastEndAndAfterClose(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	it := db.First()
	iterations := 0
	for ; it.Valid(); it.Next() {
		iterations++
	}
	require.Equal(t, len(defaultsDBKeys), iterations)
	// no panic
	it.Next()
	// still not valid
	require.False(t, it.Valid())
	require.NoError(t, it.Close())
	it.Next()
	require.False(t, it.Valid())
}

func TestBoltIterator_IteratePastBegin(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	it := db.Last()
	iterations := 0
	for ; it.Valid(); it.Prev() {
		iterations++
	}
	require.Equal(t, len(defaultsDBKeys), iterations)
	// no panic
	it.Prev()
	// still not valid
	require.False(t, it.Valid())
	require.NoError(t, it.Close())
	it.Prev()
	require.False(t, it.Valid())
}
