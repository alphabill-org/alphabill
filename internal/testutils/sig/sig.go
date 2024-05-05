package testsig

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/stretchr/testify/require"
)

func SignBytes(t *testing.T, sigData []byte) ([]byte, []byte) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	sig, err := signer.SignBytes(sigData)
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	return sig, pubKey
}

func CreateSignerAndVerifier(t *testing.T) (abcrypto.Signer, abcrypto.Verifier) {
	t.Helper()
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)
	return signer, verifier
}
