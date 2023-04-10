package twb

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseTokenTypeID(t *testing.T) {
	t.Parallel()

	t.Run("empty value, required", func(t *testing.T) {
		v, err := parseHex[TokenTypeID]("", true)
		require.EqualError(t, err, `parameter is required`)
		require.Nil(t, v)
	})

	t.Run("empty value, not required", func(t *testing.T) {
		v, err := parseHex[TokenTypeID]("", false)
		require.NoError(t, err)
		require.Nil(t, v)
	})

	t.Run("missing prefix", func(t *testing.T) {
		v, err := parseHex[TokenTypeID]("01234567890abcdef", true)
		require.EqualError(t, err, `hex string without 0x prefix`)
		require.Nil(t, v)
	})

	t.Run("not valid hex encoding", func(t *testing.T) {
		v, err := parseHex[TokenTypeID]("0xABCDEFGHIJKL", true)
		require.EqualError(t, err, `invalid hex string`)
		require.Nil(t, v)
	})

	t.Run("valid value", func(t *testing.T) {
		id := make([]byte, 33)
		n, err := rand.Read(id)
		require.NoError(t, err)
		require.EqualValues(t, len(id), n)

		v, err := parseHex[TokenTypeID](encodeHex[TokenTypeID](id), false)
		require.NoError(t, err)
		require.EqualValues(t, id, v)
	})
}

func Test_parseTokenID(t *testing.T) {
	t.Parallel()

	t.Run("empty value, required", func(t *testing.T) {
		v, err := parseHex[TokenID]("", true)
		require.EqualError(t, err, `parameter is required`)
		require.Nil(t, v)
	})

	t.Run("empty value, not required", func(t *testing.T) {
		v, err := parseHex[TokenID]("", false)
		require.NoError(t, err)
		require.Nil(t, v)
	})

	t.Run("missing prefix", func(t *testing.T) {
		v, err := parseHex[TokenID]("01234567890abcdef", true)
		require.EqualError(t, err, `hex string without 0x prefix`)
		require.Nil(t, v)
	})

	t.Run("not valid hex encoding", func(t *testing.T) {
		v, err := parseHex[TokenID]("0xABCDEFGHIJKL", true)
		require.EqualError(t, err, `invalid hex string`)
		require.Nil(t, v)
	})

	t.Run("valid value", func(t *testing.T) {
		id := make([]byte, 33)
		n, err := rand.Read(id)
		require.NoError(t, err)
		require.EqualValues(t, len(id), n)

		v, err := parseHex[TokenID](encodeHex[TokenID](id), false)
		require.NoError(t, err)
		require.EqualValues(t, id, v)
	})
}
