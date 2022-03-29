package transaction

import (
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUint256Hashing(t *testing.T) {
	// Verifies that the uint256 bytes are equals to the byte array it was made from.
	// So it doesn't matter if hash is calculated from the byte array from the uint256 byte array.
	b32 := test.RandomBytes(32)
	b32Int := uint256.NewInt(0).SetBytes(b32)
	assert.Equal(t, b32, b32Int.Bytes())

	b33 := test.RandomBytes(33)
	b33Int := uint256.NewInt(0).SetBytes(b33)
	assert.Equal(t, b33[1:], b33Int.Bytes())

	b1 := test.RandomBytes(1)
	b1Int := uint256.NewInt(0).SetBytes(b1)
	expected := [32]byte{}
	expected[31] = b1[0]
	assert.Equal(t, expected, b1Int.Bytes32())
}
