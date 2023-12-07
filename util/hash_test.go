package util

import (
	"crypto"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSum(t *testing.T) {
	t.Run("hash of nil", func(t *testing.T) {
		nilHash := Sum(crypto.SHA256, nil)
		require.NotEqual(t, Zero256, nilHash)
	})
	t.Run("empty hash", func(t *testing.T) {
		nilHash := Sum(crypto.SHA256)
		require.NotEqual(t, Zero256, nilHash)
	})
	t.Run("hash of string - test", func(t *testing.T) {
		hash, err := hex.DecodeString("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
		// calculate hash from separate parts
		hashSum := Sum(crypto.SHA256, []byte("te"), []byte("st"))
		require.NoError(t, err)
		require.Equal(t, hash, hashSum)
	})
}

func TestSum256(t *testing.T) {
	t.Run("hash of nil - returns zero hash a.k.a. slice with 32 bytes set to 0", func(t *testing.T) {
		require.Equal(t, Zero256, Sum256(nil))
	})
	t.Run("hash of string - test", func(t *testing.T) {
		hash, err := hex.DecodeString("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08")
		require.NoError(t, err)
		require.Equal(t, hash, Sum256([]byte("test")))
	})
}

func TestSum512(t *testing.T) {
	t.Run("hash of nil - returns zero hash a.k.a. slice with 64 bytes set to 0", func(t *testing.T) {
		require.Equal(t, Zero512, Sum512(nil))
	})
	t.Run("hash of string - test", func(t *testing.T) {
		hash, err := hex.DecodeString("ee26b0dd4af7e749aa1a8ee3c10ae9923f618980772e473f8819a5d4940e0db27ac185f8a0e1d5f84f88bc887fd67b143732c304cc5fa9ad8e6f57f50028a8ff")
		require.NoError(t, err)
		require.Equal(t, hash, Sum512([]byte("test")))
	})
}
