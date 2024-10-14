package testsig

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

// NewAuthProofSignature creates a P2PKH predicate signature for AuthProof.
func NewAuthProofSignature(t *testing.T, txo *types.TransactionOrder, signer abcrypto.Signer) []byte {
	sigBytes, err := txo.AuthProofSigBytes()
	require.NoError(t, err)
	return NewP2pkhSignature(t, signer, sigBytes)
}

// NewFeeProofSignature creates a P2PKH fee predicate signature for FeeProof.
func NewFeeProofSignature(t *testing.T, txo *types.TransactionOrder, signer abcrypto.Signer) []byte {
	sigBytes, err := txo.FeeProofSigBytes()
	require.NoError(t, err)
	return NewP2pkhSignature(t, signer, sigBytes)
}

// NewStateLockProofSignature creates a P2PKH fee predicate signature for StateLock.
func NewStateLockProofSignature(t *testing.T, txo *types.TransactionOrder, signer abcrypto.Signer) []byte {
	sigBytes, err := txo.StateLockProofSigBytes()
	require.NoError(t, err)
	return NewP2pkhSignature(t, signer, sigBytes)
}

func NewP2pkhSignature(t *testing.T, signer abcrypto.Signer, sigBytes []byte) []byte {
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
