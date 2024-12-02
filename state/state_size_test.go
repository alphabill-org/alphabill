package state

import (
	"errors"
	"hash"
	"testing"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
)

func Test_stateSize(t *testing.T) {
	t.Skip("different approach for calculating state size is needed (AB-1789)")

	t.Run("empty state", func(t *testing.T) {
		s := NewEmptyState()
		size, err := s.Size()
		require.NoError(t, err)
		require.Zero(t, size)
	})

	t.Run("empty owner", func(t *testing.T) {
		s := NewEmptyState()
		// owner is nil, 10 bytes of data
		require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 1}, &ud{writeRandomBytes(10)})))
		size, err := s.Size()
		require.NoError(t, err)
		require.EqualValues(t, 10, size)
		// owner is empty slice, 12 bytes of data
		require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 0, 2}, &ud{writeRandomBytes(12)})))
		size, err = s.Size()
		require.NoError(t, err)
		require.EqualValues(t, 10+12, size)
	})

	t.Run("zero length data", func(t *testing.T) {
		s := NewEmptyState()
		// 0 bytes of data
		require.NoError(t, s.Apply(AddUnit([]byte{0, 0, 1, 1}, &ud{write: func(_ hash.Hash) error { return nil }})))
		size, err := s.Size()
		require.NoError(t, err)
		require.EqualValues(t, 0, size)
	})

	t.Run("size of multiple units", func(t *testing.T) {
		s := NewEmptyState()
		// four times 10 byte data
		require.NoError(t, s.Apply(
			AddUnit([]byte{0, 0, 0, 1}, &ud{writeRandomBytes(10)}),
			AddUnit([]byte{0, 0, 0, 2}, &ud{writeRandomBytes(10)}),
			AddUnit([]byte{0, 0, 0, 3}, &ud{writeRandomBytes(10)}),
			AddUnit([]byte{0, 0, 0, 4}, &ud{writeRandomBytes(10)}),
		))
		size, err := s.Size()
		require.NoError(t, err)
		require.EqualValues(t, 4*10, size)
	})

	t.Run("data write error", func(t *testing.T) {
		s := NewEmptyState()
		expErr := errors.New("writing data")
		require.NoError(t, s.Apply(
			AddUnit([]byte{0, 0, 0, 2}, &ud{writeRandomBytes(10)}),
			AddUnit([]byte{0, 0, 0, 1}, &ud{write: func(_ hash.Hash) error { return expErr }}),
			AddUnit([]byte{0, 0, 0, 3}, &ud{writeRandomBytes(10)}),
		))

		size, err := s.Size()
		require.ErrorIs(t, err, expErr, "with size %d", size)
	})
}

// mock unit data
type ud struct {
	write func(h hash.Hash) error
}

func (t *ud) Write(h abhash.Hasher) {
	h.Write(t)
}

func (t *ud) SummaryValueInput() uint64 {
	return 0
}

func (t *ud) Copy() types.UnitData {
	return &ud{write: t.write}
}

func (t *ud) Owner() []byte {
	return nil
}

// create Write implementation for "ud" which writes "count" random bytes
func writeRandomBytes(count int) func(h hash.Hash) error {
	return func(h hash.Hash) error {
		_, err := h.Write(test.RandomBytes(count))
		return err
	}
}
