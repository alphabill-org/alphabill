package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertKey(t *testing.T) {
	// appends to prefix
	require.Equal(t, []byte("cert_test"), certKey([]byte("test")))
	require.Equal(t, []byte("cert_00000001"), certKey([]byte("00000001")))
	// nil case
	require.Equal(t, []byte(certPrefix), certKey(nil))
}

func TestBlockKey(t *testing.T) {
	const round uint64 = 1
	require.Equal(t, []byte("block_\000\000\000\000\000\000\000\001"), blockKey(round))
}
