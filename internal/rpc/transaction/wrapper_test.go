package transaction

import (
	"crypto"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUint256Hashing(t *testing.T) {
	// Verifies that the uint256 bytes are equals to the byte array it was made from.
	// So it doesn't matter if hash is calculated from the byte array from the uint256 byte array.
	b32 := test.RandomBytes(32)
	b32Int := uint256.NewInt(0).SetBytes(b32)
	bytes32 := b32Int.Bytes32()
	assert.Equal(t, b32, bytes32[:])

	b33 := test.RandomBytes(33)
	b33Int := uint256.NewInt(0).SetBytes(b33)
	bytes32 = b33Int.Bytes32()
	assert.Equal(t, b33[1:], bytes32[:])

	b1 := test.RandomBytes(1)
	b1Int := uint256.NewInt(0).SetBytes(b1)
	expected := [32]byte{}
	expected[31] = b1[0]
	assert.Equal(t, expected, b1Int.Bytes32())
}

func TestUint256Hashing_LeadingZeroByte(t *testing.T) {
	b32 := test.RandomBytes(32)
	b32[0] = 0x00
	b32Int := uint256.NewInt(0).SetBytes(b32)
	b32IntBytes := b32Int.Bytes32() // Bytes32() works, Bytes() does not

	hasher := crypto.SHA256.New()
	hasher.Write(b32IntBytes[:])
	h1 := hasher.Sum(nil)

	hasher.Reset()
	hasher.Write(b32)
	h2 := hasher.Sum(nil)

	require.EqualValues(t, h2, h1)
}
