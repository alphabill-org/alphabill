package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Unit_IsStateLocked(t *testing.T) {
	t.Run("state lock is not set", func(t *testing.T) {
		u := &UnitV1{}
		require.False(t, u.IsStateLocked())
	})

	t.Run("state lock is set", func(t *testing.T) {
		u := &UnitV1{stateLockTx: []byte{1}}
		require.True(t, u.IsStateLocked())
	})
}

func Test_Unit_StateLockTx(t *testing.T) {
	t.Run("state lock is not set", func(t *testing.T) {
		u := &UnitV1{}
		require.Nil(t, u.StateLockTx())
	})

	t.Run("state lock is set", func(t *testing.T) {
		u := &UnitV1{stateLockTx: []byte{1}}
		require.Equal(t, []byte{1}, u.StateLockTx())
	})
}

func Test_Unit_latestStateLockTx(t *testing.T) {
	t.Run("no logs and no state lock", func(t *testing.T) {
		u := &UnitV1{}
		require.Nil(t, u.latestStateLockTx())
	})

	t.Run("no logs but state lock is set", func(t *testing.T) {
		u := &UnitV1{stateLockTx: []byte{1}}
		require.Equal(t, []byte{1}, u.latestStateLockTx())
	})

	t.Run("logs exist but no state lock in the latest log", func(t *testing.T) {
		u := &UnitV1{
			logs: []*Log{
				{NewStateLockTx: nil},
			},
		}
		require.Nil(t, u.latestStateLockTx())
	})

	t.Run("logs exist and state lock is set in the latest log", func(t *testing.T) {
		u := &UnitV1{
			logs: []*Log{
				{NewStateLockTx: []byte{1}},
			},
		}
		require.Equal(t, []byte{1}, u.latestStateLockTx())
	})
}
