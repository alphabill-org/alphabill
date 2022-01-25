package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SigningTestSuite struct {
	suite.Suite
}

func TestSigningTestSuite(t *testing.T) {
	suite.Run(t, new(SigningTestSuite))
}

func (s *SigningTestSuite) Test_InvalidPrivateKeySizes() {
	signer, err := NewInMemorySecp256K1SignerFromKey(nil)
	require.Error(s.T(), err)
	require.Nil(s.T(), signer)

	signer2, err := NewInMemorySecp256K1SignerFromKey(make([]byte, 33))
	require.Error(s.T(), err)
	require.Nil(s.T(), signer2)
}

func (s *SigningTestSuite) Test_VerifierFromSigner() {
	signer, err := NewInMemorySecp256K1Signer()
	require.NoError(s.T(), err)

	verifier, err := signer.Verifier()
	require.NoError(s.T(), err)
	s.assertSignAndVerify(signer, verifier)
}

func (s *SigningTestSuite) Test_VerifierFromKeyBytes() {
	signer, err := NewInMemorySecp256K1Signer()
	require.NoError(s.T(), err)

	verifier1, err := signer.Verifier()
	require.NoError(s.T(), err)
	pubkey, err := verifier1.MarshalPublicKey()
	require.NoError(s.T(), err)
	require.Len(s.T(), pubkey, CompressedSecp256K1PublicKeySize, "pubkey length is not expected compressed key size")

	verifier, err := NewVerifierSecp256k1(pubkey)
	require.NoError(s.T(), err)
	s.assertSignAndVerify(signer, verifier)

	// Try to marshal public again from verifier that is created from compressed key
	pubkeyAgain, err := verifier.MarshalPublicKey()
	require.NoError(s.T(), err)
	require.Len(s.T(), pubkeyAgain, CompressedSecp256K1PublicKeySize, "pubkey length is not expected compressed key size")

	verifierAgain, err := NewVerifierSecp256k1(pubkeyAgain)
	require.NoError(s.T(), err)
	s.assertSignAndVerify(signer, verifierAgain)
}

func (s *SigningTestSuite) Test_MarshallingPrivateKey() {
	signer, err := NewInMemorySecp256K1Signer()
	require.NoError(s.T(), err)

	privKey, err := signer.MarshalPrivateKey()
	require.NoError(s.T(), err)

	signerFromKey, err := NewInMemorySecp256K1SignerFromKey(privKey)
	require.NoError(s.T(), err)

	verifier, err := signerFromKey.Verifier()
	require.NoError(s.T(), err)

	s.assertSignAndVerify(signerFromKey, verifier)
}

func TestSignerNilArguments(t *testing.T) {
	var signer InMemorySecp256K1Signer

	bytes, err := signer.SignBytes([]byte{1, 2, 3})
	require.Error(t, err)
	require.Nil(t, bytes)
}

func TestSignerNilData(t *testing.T) {
	signer, err := NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	bytes, err := signer.SignBytes(nil)
	require.Error(t, err)
	require.Nil(t, bytes)
}

func TestVerifierNilVerifier(t *testing.T) {
	var verifier verifierSecp256k1

	err := verifier.VerifyBytes([]byte{1}, []byte{2})
	require.Error(t, err)

	key, err := verifier.MarshalPublicKey()
	require.Error(t, err)
	require.Nil(t, key)
}

func TestVerifierIllegalInput(t *testing.T) {
	signer, err := NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)

	data := []byte{1, 2, 3, 4}
	sig, err := signer.SignBytes(data)
	require.NoError(t, err)

	err = verifier.VerifyBytes([]byte{1, 2}, data)
	require.Error(t, err, "verifying signature with illegal size must fail")

	err = verifier.VerifyBytes(sig, append(data, 5))
	require.Error(t, err, "verifying not matching data and signature must fail")
}

func (s *SigningTestSuite) assertSignAndVerify(signer Signer, verifier Verifier) {
	signAndVerifyBytes(s.T(), signer, verifier)
	signAndVerifyNoRecoveryID(s.T(), signer, verifier)
}

func signAndVerifyNoRecoveryID(t *testing.T, signer Signer, verifier Verifier) {
	data := []byte{1, 2, 3}
	sig, err := signer.SignBytes(data)
	require.NoError(t, err)

	sigWithoutRecoveryID := sig[:len(sig)-1]

	err = verifier.VerifyBytes(sigWithoutRecoveryID, data)
	require.NoError(t, err)
}

func signAndVerifyBytes(t *testing.T, signer Signer, verifier Verifier) {
	data := []byte{1, 2, 3}
	sig, err := signer.SignBytes(data)
	require.NoError(t, err)

	err = verifier.VerifyBytes(sig, data)
	require.NoError(t, err)
}
