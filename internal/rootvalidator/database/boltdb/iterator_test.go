package boltdb

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

var defaultsDBKeys = []string{"1", "2", "3", "4"}

func initDB(t *testing.T, defaults []string) *BoltDB {
	t.Helper()
	f, err := os.CreateTemp("", "bolt-*.db")
	if err != nil {
		t.Fatal(err)
	}
	boltDB, err := New(f.Name())
	require.NoError(t, err)
	require.NotNil(t, boltDB)
	if defaults == nil {
		return boltDB
	}
	// init with default values
	for idx, key := range defaults {
		require.NoError(t, boltDB.Write([]byte(key), strconv.Itoa(idx)))
	}
	return boltDB
}

func TestBoltIterator_CloseNil(t *testing.T) {
	it := &Itr{}
	require.NoError(t, it.Close())
}

func TestBoltIterator_NewIterator(t *testing.T) {
	it := NewIterator(nil, []byte(""), json.Unmarshal)
	require.False(t, it.Valid())
}

func TestBoltIterator_newIteratorNil(t *testing.T) {
	it, err := newIterator(nil, []byte(""), json.Unmarshal)
	require.Error(t, err)
	require.Nil(t, it)
}

func TestBoltIterator_TestEmptyDB(t *testing.T) {
	db := initDB(t, nil)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	it := db.First()
	defer func() {
		require.NoError(t, it.Close())
	}()
	require.False(t, it.Valid())
	var value string
	require.ErrorContains(t, it.Value(value), "unexpected end of JSON input")
	require.Len(t, it.Key(), 0)
}

func TestPersistentStore_TestIterator(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	it := db.First()
	defer func() {
		require.NoError(t, it.Close())
	}()
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

func TestBoltIterator_TestIteratorReverse(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	it := db.Last()
	defer func() {
		require.NoError(t, it.Close())
	}()
	require.True(t, it.Valid())
	require.Equal(t, it.Key(), []byte("4"))
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

func TestBoltIterator_TestIteratorSeek(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	it := db.Find([]byte("3"))
	defer func() {
		require.NoError(t, it.Close())
	}()
	require.True(t, it.Valid())
	require.Equal(t, []byte("3"), it.Key())
	var value string
	require.NoError(t, it.Value(&value))
	// default value is one less
	require.Equal(t, "2", value)
}

func TestBoltIterator_FindNoMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	// seek past end
	it := db.Find([]byte("waypastend"))
	defer func() {
		require.NoError(t, it.Close())
	}()
	require.False(t, it.Valid())
}

func TestBoltIterator_FindClosestMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	// seek past end
	it := db.Find([]byte("0"))
	defer func() {
		require.NoError(t, it.Close())
	}()
	require.True(t, it.Valid())
	require.Equal(t, []byte("1"), it.Key())
}

func TestBoltIterator_DoubleClose(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	// seek past end
	it := db.Find([]byte("0"))
	require.NoError(t, it.Close())
	require.NoError(t, it.Close())
}

func TestBoltIterator_IteratePastEndAndAfterClose(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
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
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
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
