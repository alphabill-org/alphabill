package storage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInMemoryDB(t *testing.T) {
	// creates an in memory db
	db, err := New("")
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NotNil(t, db.GetCertificatesDB())
	require.NotNil(t, db.GetBlocksDB())
	require.NotNil(t, db.GetRootDB())
}

func TestNewInPersistentDBPathNotDir(t *testing.T) {
	file, err := os.CreateTemp("", "bolt-*.db")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Remove(file.Name()))
	}()
	// creates a bolt DB
	db, err := New(file.Name())
	require.ErrorContains(t, err, "not a directory")
	require.Nil(t, db)
}

func TestNewInPersistentDB(t *testing.T) {
	dir, err := os.MkdirTemp("", "bolt*")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()
	// creates a bolt DB
	db, err := New(dir)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NotNil(t, db.GetCertificatesDB())
	require.NotNil(t, db.GetBlocksDB())
	require.NotNil(t, db.GetRootDB())
}
