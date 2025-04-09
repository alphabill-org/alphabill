package txsystem

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestETBuffer(t *testing.T) {
	txID := "tx1"
	txTimeout := uint64(10)

	t.Run("add and get ok", func(t *testing.T) {
		buffer := NewETBuffer()

		timeout, f := buffer.Get(txID)
		require.False(t, f)
		require.Zero(t, timeout)

		buffer.Add(txID, txTimeout)

		timeout, f = buffer.Get(txID)
		require.True(t, f)
		require.Equal(t, txTimeout, timeout)
	})

	t.Run("revert and commit ok", func(t *testing.T) {
		buffer := NewETBuffer()
		txID1 := "tx1"
		txID2 := "tx2"

		buffer.Add(txID1, txTimeout)
		buffer.Commit()
		buffer.Add(txID2, txTimeout)
		buffer.Revert()

		// verify tx1 exists after revert
		timeout, f := buffer.Get(txID1)
		require.True(t, f)
		require.Equal(t, txTimeout, timeout)

		// verify tx2 does not exist after revert
		timeout, f = buffer.Get(txID2)
		require.False(t, f)
		require.Zero(t, timeout)
	})

	t.Run("clear expired transactions", func(t *testing.T) {
		buffer := NewETBuffer()
		txID1 := "tx1"
		txID2 := "tx2"
		txID3 := "tx3"
		timeout1 := uint64(1)
		timeout2 := uint64(2)
		timeout3 := uint64(3)

		buffer.Add(txID1, timeout1)
		buffer.Add(txID2, timeout2)
		buffer.Add(txID3, timeout3)
		buffer.Commit()
		buffer.ClearExpired(2) // should clear tx1 and tx2

		timeout, f := buffer.Get(txID1)
		require.False(t, f)
		require.Zero(t, timeout)

		timeout, f = buffer.Get(txID2)
		require.False(t, f)
		require.Zero(t, timeout)

		timeout, f = buffer.Get(txID3)
		require.True(t, f)
		require.Equal(t, timeout3, timeout)
	})

	t.Run("hash", func(t *testing.T) {
		buffer1 := NewETBuffer()
		buffer1.Add("tx1", 1)
		buffer1.Add("tx2", 2)
		buffer1.Add("tx3", 3)
		buffer1.Commit()

		buffer2 := NewETBuffer()
		buffer2.Add("tx3", 3)
		buffer2.Add("tx2", 2)
		buffer2.Add("tx1", 1)
		buffer2.Commit()

		buffer1Hash, err := buffer1.Hash()
		require.NoError(t, err)

		buffer2Hash, err := buffer2.Hash()
		require.NoError(t, err)

		require.Equal(t, buffer1Hash, buffer2Hash)
	})
}
