package canonicalizer

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUint64ToBytes(t *testing.T) {
	assert.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 7}, Uint64ToBytes(7))
	assert.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, Uint64ToBytes(0))
	assert.Equal(t, []byte{255, 255, 255, 255, 255, 255, 255, 255}, Uint64ToBytes(math.MaxUint64))
}

func TestBytesToUint64(t *testing.T) {
	assert.Equal(t, uint64(7), BytesToUint64([]byte{0, 0, 0, 0, 0, 0, 0, 7}))
	assert.Equal(t, uint64(0), BytesToUint64([]byte{0, 0, 0, 0, 0, 0, 0, 0}))
	assert.Equal(t, uint64(math.MaxUint64), BytesToUint64([]byte{255, 255, 255, 255, 255, 255, 255, 255}))
}
