package testsig

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
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

// NewOwnerProof creates a P2PKH predicate signature aka the "OwnerProof"
func NewOwnerProof(t *testing.T, txo *types.TransactionOrder, signer abcrypto.Signer) []byte {
	sigBytes, err := txo.PayloadBytes()
	require.NoError(t, err)
	signedBytes, err := signer.SignBytes(sigBytes)
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return templates.NewP2pkh256SignatureBytes(signedBytes, publicKey)
}

// NewFeeProof creates a P2PKH fee predicate signature aka the "FeeProof"
func NewFeeProof(t *testing.T, txo *types.TransactionOrder, signer abcrypto.Signer) []byte {
	sigBytes, err := txo.FeeProofSigBytes()
	require.NoError(t, err)
	signedBytes, err := signer.SignBytes(sigBytes)
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return templates.NewP2pkh256SignatureBytes(signedBytes, publicKey)
}

func CreateSignerAndVerifier(t *testing.T) (abcrypto.Signer, abcrypto.Verifier) {
	t.Helper()
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)
	return signer, verifier
}
