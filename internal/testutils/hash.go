package test

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"
)

type Hashable interface {
	Hash(hashAlgorithm crypto.Hash) ([]byte, error)
}

func DoHash(t *testing.T, data Hashable) []byte {
	t.Helper()
	h, err := data.Hash(crypto.SHA256)
	require.NoError(t, err)
	return h
}
