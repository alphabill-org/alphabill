package boltdb

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoltTx_Nil(t *testing.T) {
	tx, err := NewBoltTx(nil, []byte("test"), json.Marshal)
	require.Error(t, err)
	require.Nil(t, tx)
}

func TestBoltTx_StartAndCommit(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Commit())
	// still empty
	require.True(t, db.Empty())
}

func TestBoltTx_StartAndRollback(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Rollback())
	// still empty
	require.True(t, db.Empty())
}

func TestBoltTx_SimpleCommit(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	require.True(t, db.Empty())
	require.NoError(t, tx.Commit())
	require.False(t, db.Empty())
	var res string
	f, err := db.Read([]byte("test1"), &res)
	require.NoError(t, err)
	require.True(t, f)
	require.Equal(t, "1", res)
	f, err = db.Read([]byte("test2"), &res)
	require.NoError(t, err)
	require.True(t, f)
	require.Equal(t, "2", res)
	f, err = db.Read([]byte("test3"), &res)
	require.NoError(t, err)
	require.True(t, f)
	require.Equal(t, "3", res)
}

func TestBoltTx_SimpleRollback(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	require.True(t, db.Empty())
	require.NoError(t, tx.Rollback())
	require.True(t, db.Empty())
	// write a single value
	require.NoError(t, db.Write([]byte("test"), "test"))
	require.False(t, db.Empty())
}

func TestBoltTx_Delete(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	// delete non-existing key
	require.NoError(t, tx.Delete([]byte("test")))
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	// delete test2
	require.NoError(t, tx.Delete([]byte("test2")))
	require.True(t, db.Empty())
	require.NoError(t, tx.Commit())
	require.False(t, db.Empty())
	var res string
	f, err := db.Read([]byte("test1"), &res)
	require.NoError(t, err)
	require.True(t, f)
	require.Equal(t, "1", res)
	f, err = db.Read([]byte("test2"), &res)
	require.NoError(t, err)
	require.False(t, f)
	f, err = db.Read([]byte("test3"), &res)
	require.NoError(t, err)
	require.True(t, f)
	require.Equal(t, "3", res)
}

func TestBoltTx_WriteEncodeError(t *testing.T) {
	db := initBoltDB(t)
	defer func() {
		require.NoError(t, os.Remove(db.Path()))
	}()
	require.True(t, db.Empty())
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	c := make(chan int)
	require.Error(t, tx.Write([]byte("channel"), c))
	require.NoError(t, tx.Rollback())
}
