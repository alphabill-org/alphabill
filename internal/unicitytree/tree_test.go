package unicitytree

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUnicityTree(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: []byte{0x00, 0x00, 0x00, 0x01}, inputRecord: InputRecord{
		value: []byte{0x01},
	}}})
	require.NoError(t, err)
	require.NotNil(t, unicityTree)
}

func TestGetCertificate_Ok(t *testing.T) {
	key := []byte{0x00, 0x00, 0x00, 0x01}
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: key, inputRecord: InputRecord{
		value: []byte{0x01},
	}}})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate(key)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, key, cert.systemIdentifier)
	require.Equal(t, systemIdentifierLength*8, len(cert.siblingHashes))
}

func TestGetCertificate_InvalidKey(t *testing.T) {
	unicityTree, err := New(sha256.New(), []*Data{{systemIdentifier: []byte{0x00, 0x00, 0x00, 0x01}, inputRecord: InputRecord{
		value: []byte{0x01},
	}}})
	require.NoError(t, err)
	cert, err := unicityTree.GetCertificate([]byte{0x00, 0x00})

	require.Nil(t, cert)
	require.ErrorIs(t, err, ErrInvalidSystemIdentifierLength)
}
