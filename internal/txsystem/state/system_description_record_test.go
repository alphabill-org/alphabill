package state

import (
	"crypto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemDescriptionRecord_CanBeCreated(t *testing.T) {
	sdr := NewSystemDescriptionRecord("ab")
	require.NotNil(t, sdr)
}

func TestSystemDescriptionRecord_CanBeHashed(t *testing.T) {
	sdr := NewSystemDescriptionRecord("ab")

	hasher := crypto.SHA256.New()
	sdr.AddToHasher(hasher)
	actualHash := hasher.Sum(nil)

	hasher.Reset()
	hasher.Write([]byte("ab"))
	expectedHash := hasher.Sum(nil)

	require.EqualValues(t, expectedHash, actualHash)
}
