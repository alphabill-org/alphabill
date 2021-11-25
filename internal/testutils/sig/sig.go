package testsig

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"github.com/stretchr/testify/require"
	"testing"
)

func SignBytes(t *testing.T, sigData []byte) ([]byte, []byte) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	sig, err := signer.SignBytes(sigData)
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	return sig, pubKey
}
