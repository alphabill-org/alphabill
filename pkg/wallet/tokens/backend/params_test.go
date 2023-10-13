package backend

import (
	"crypto/rand"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/stretchr/testify/require"
)

func Test_parseTokenTypeID(t *testing.T) {
	t.Parallel()

	t.Run("empty value, required", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("", true)
		require.EqualError(t, err, `parameter is required`)
		require.Nil(t, v)
	})

	t.Run("empty value, not required", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("", false)
		require.NoError(t, err)
		require.Nil(t, v)
	})

	t.Run("missing prefix", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("01234567890abcdef", true)
		require.EqualError(t, err, `hex string without 0x prefix`)
		require.Nil(t, v)
	})

	t.Run("not valid hex encoding", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("0xABCDEFGHIJKL", true)
		require.EqualError(t, err, `invalid hex string`)
		require.Nil(t, v)
	})

	t.Run("valid value", func(t *testing.T) {
		id := make([]byte, 33)
		n, err := rand.Read(id)
		require.NoError(t, err)
		require.EqualValues(t, len(id), n)

		v, err := sdk.ParseHex[types.UnitID](sdk.EncodeHex(id), false)
		require.NoError(t, err)
		require.EqualValues(t, id, v)
	})
}

func Test_parseTokenID(t *testing.T) {
	t.Parallel()

	t.Run("empty value, required", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("", true)
		require.EqualError(t, err, `parameter is required`)
		require.Nil(t, v)
	})

	t.Run("empty value, not required", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("", false)
		require.NoError(t, err)
		require.Nil(t, v)
	})

	t.Run("missing prefix", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("01234567890abcdef", true)
		require.EqualError(t, err, `hex string without 0x prefix`)
		require.Nil(t, v)
	})

	t.Run("not valid hex encoding", func(t *testing.T) {
		v, err := sdk.ParseHex[types.UnitID]("0xABCDEFGHIJKL", true)
		require.EqualError(t, err, `invalid hex string`)
		require.Nil(t, v)
	})

	t.Run("valid value", func(t *testing.T) {
		id := make([]byte, 33)
		n, err := rand.Read(id)
		require.NoError(t, err)
		require.EqualValues(t, len(id), n)

		v, err := sdk.ParseHex[types.UnitID](sdk.EncodeHex(id), false)
		require.NoError(t, err)
		require.EqualValues(t, id, v)
	})
}
