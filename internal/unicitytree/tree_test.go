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
