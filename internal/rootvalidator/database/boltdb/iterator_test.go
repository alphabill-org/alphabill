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
	defer os.Remove(db.Path())
	it := db.First()
	defer it.Close()
	require.False(t, it.Valid())
	var value string
	require.ErrorContains(t, it.Value(value), "unexpected end of JSON input")
	require.Len(t, it.Key(), 0)
}

func TestPersistentStore_TestIterator(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer os.Remove(db.Path())
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

func TestBoltIterator_TestIteratorReverse(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer os.Remove(db.Path())
	it := db.Last()
	defer it.Close()
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
	defer os.Remove(db.Path())
	it := db.Find([]byte("3"))
	defer it.Close()
	require.True(t, it.Valid())
	require.Equal(t, []byte("3"), it.Key())
	var value string
	require.NoError(t, it.Value(&value))
	// default value is one less
	require.Equal(t, "2", value)
}

func TestBoltIterator_FindNoMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer os.Remove(db.Path())
	// seek past end
	it := db.Find([]byte("waypastend"))
	defer it.Close()
	require.False(t, it.Valid())
}

func TestBoltIterator_FindClosestMatch(t *testing.T) {
	db := initDB(t, defaultsDBKeys)
	defer os.Remove(db.Path())
	// seek past end
	it := db.Find([]byte("0"))
	defer it.Close()
	require.True(t, it.Valid())
	require.Equal(t, []byte("1"), it.Key())
}
