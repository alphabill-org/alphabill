package tokens

import (
	gocrypto "crypto"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTokenTxSystem_DefaultOptions(t *testing.T) {
	txs, err := New()
	require.NoError(t, err)
	require.Equal(t, gocrypto.SHA256, txs.hashAlgorithm)
	require.Equal(t, DefaultTokenTxSystemIdentifier, txs.systemIdentifier)

	require.NotNil(t, txs.state)
	state, err := txs.State()
	require.NoError(t, err)
	require.Equal(t, make([]byte, gocrypto.SHA256.Size()), state.Root())
	require.Equal(t, zeroSummaryValue.Bytes(), state.Summary())
}

func TestNewTokenTxSystem_NilSystemIdentifier(t *testing.T) {
	txs, err := New(WithSystemIdentifier(nil))
	require.ErrorContains(t, err, ErrStrSystemIdentifierIsNil)
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_UnsupportedHashAlgorithm(t *testing.T) {
	txs, err := New(WithHashAlgorithm(gocrypto.SHA1))
	require.ErrorContains(t, err, "invalid hash algorithm")
	require.Nil(t, txs)
}

func TestNewTokenTxSystem_OverrideDefaultOptions(t *testing.T) {
	systemIdentifier := []byte{0, 0, 0, 7}
	txs, err := New(WithSystemIdentifier(systemIdentifier), WithHashAlgorithm(gocrypto.SHA512))
	require.NoError(t, err)
	require.Equal(t, gocrypto.SHA512, txs.hashAlgorithm)
	require.Equal(t, systemIdentifier, txs.systemIdentifier)

	require.NotNil(t, txs.state)
	state, err := txs.State()
	require.NoError(t, err)
	require.Equal(t, make([]byte, gocrypto.SHA512.Size()), state.Root())
	require.Equal(t, zeroSummaryValue.Bytes(), state.Summary())
}
