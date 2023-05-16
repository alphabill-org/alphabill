package boltdb

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoltTx_Nil(t *testing.T) {
	tx, err := NewBoltTx(nil, []byte("test"), json.Marshal, json.Unmarshal)
	require.Error(t, err)
	require.Nil(t, tx)
}

func TestBoltTx_StartAndCommit(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Commit())
	// still empty
	require.True(t, isEmpty(t, db))
}

func TestBoltTx_StartAndRollback(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Rollback())
	// still empty
	require.True(t, isEmpty(t, db))
}

func TestBoltTx_SimpleCommit(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	require.True(t, isEmpty(t, db))
	require.NoError(t, tx.Commit())
	require.False(t, isEmpty(t, db))
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

func TestBoltTx_UseAfterClose(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	var val string
	found, err := tx.Read([]byte("test2"), &val)
	require.True(t, found)
	require.NoError(t, err, "tx closed")
	require.NoError(t, tx.Delete([]byte("test2")))
	require.NoError(t, tx.Commit())
	found, err = tx.Read([]byte("test2"), &val)
	require.False(t, found)
	require.ErrorContains(t, err, "tx closed")
	require.ErrorContains(t, tx.Write([]byte("test1"), "1"), "tx closed")
	require.ErrorContains(t, tx.Delete([]byte("test1")), "tx closed")
}

func TestBoltTx_SimpleRollback(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	var val string
	found, err := tx.Read([]byte("test"), &val)
	require.False(t, found)
	require.NoError(t, err)
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	// read value
	found, err = tx.Read([]byte("test2"), &val)
	require.True(t, found)
	require.NoError(t, err)
	require.Equal(t, "2", val)
	require.True(t, isEmpty(t, db))
	require.NoError(t, tx.Rollback())
	require.True(t, isEmpty(t, db))
	// write a single value
	require.NoError(t, db.Write([]byte("test"), "test"))
	require.False(t, isEmpty(t, db))
}

func TestBoltTx_Delete(t *testing.T) {
	db := initBoltDB(t)
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	// delete non-existing key
	require.NoError(t, tx.Delete([]byte("test")))
	require.NoError(t, tx.Write([]byte("test1"), "1"))
	require.NoError(t, tx.Write([]byte("test2"), "2"))
	require.NoError(t, tx.Write([]byte("test3"), "3"))
	var val string
	found, err := tx.Read([]byte("test2"), &val)
	require.True(t, found)
	require.NoError(t, err)
	// delete test2
	require.NoError(t, tx.Delete([]byte("test2")))
	require.True(t, isEmpty(t, db))
	found, err = tx.Read([]byte("test2"), &val)
	require.False(t, found)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.False(t, isEmpty(t, db))
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
	require.True(t, isEmpty(t, db))
	tx, err := db.StartTx()
	require.NoError(t, err)
	require.NotNil(t, tx)
	c := make(chan int)
	require.Error(t, tx.Write([]byte("channel"), c))
	require.NoError(t, tx.Rollback())
}
