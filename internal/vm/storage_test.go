package vm

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryStorage_ReadWrite(t *testing.T) {
	var key = "test"
	var testValue = "value"
	storage := NewMemoryStorage()
	vBytes, err := storage.Read([]byte(key))
	require.ErrorContains(t, err, "state file not found")
	require.Nil(t, vBytes)
	// add a value
	err = storage.Write([]byte(key), []byte(testValue))
	require.NoError(t, err)
	// read again, get updated value
	vBytes, err = storage.Read([]byte(key))
	require.NoError(t, err)
	require.True(t, bytes.Equal(vBytes, []byte(testValue)))
}

func TestMemoryStorage_ReadWrite_Nil(t *testing.T) {
	var testValue = "value"
	storage := NewMemoryStorage()
	vBytes, err := storage.Read(nil)
	require.ErrorContains(t, err, "invalid key")
	require.Nil(t, vBytes)
	// add a value
	err = storage.Write(nil, []byte(testValue))
	require.ErrorContains(t, err, "invalid key")
}
