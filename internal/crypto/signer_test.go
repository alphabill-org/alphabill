package crypto

import (
	"crypto/ed25519"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain/canonicalizer"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Canonicalizable struct
type (
	A struct {
		Content   []byte `hsh:"idx=1"`
		Signature []byte `hsh:"idx=2"` // must be excluded from signature
	}

	SigningTestSuite struct {
		suite.Suite
	}
)

func (a A) Canonicalize() ([]byte, error) {
	return a.Content, nil
}

func TestSigningTestSuite(t *testing.T) {
	suite.Run(t, new(SigningTestSuite))
}

func (s *SigningTestSuite) SetupTest() {
	canonicalizer.RegisterTemplate((*A)(nil))
}

func (s *SigningTestSuite) Test_VerifierFromObject() {
	signer, err := NewInMemoryEd25519Signer()
	require.NoError(s.T(), err)

	s.assertSignAndVerify(signer, signer.Verifier())
}

func (s *SigningTestSuite) Test_VerifierFromKeyBytes() {
	signer, err := NewInMemoryEd25519Signer()
	require.NoError(s.T(), err)

	verifier := NewEd25519Verifier(signer.key.Public().(ed25519.PublicKey))

	s.assertSignAndVerify(signer, verifier)
}

func (s *SigningTestSuite) Test_MarshallingPrivateKey() {
	signer, err := NewInMemoryEd25519Signer()
	require.NoError(s.T(), err)

	seed, err := signer.MarshalPrivateKey()
	require.NoError(s.T(), err)

	signerFromSeed := NewInMemoryEd25519SignerFromSeed(seed)

	s.assertSignAndVerify(signer, signerFromSeed.Verifier())
}

func (s *SigningTestSuite) assertSignAndVerify(signer Signer, verifier Verifier) {
	signAndVerifyBytes(s.T(), signer, verifier)
	signAndVerifyObject(s.T(), signer, verifier)
}

func signAndVerifyBytes(t *testing.T, signer Signer, verifier Verifier) {
	data := []byte{1, 2, 3}
	sigBytes, err := signer.SignBytes(data)
	require.NoError(t, err)

	err = verifier.VerifyBytes(sigBytes, data)
	require.NoError(t, err)
}

func signAndVerifyObject(t *testing.T, signer Signer, verifier Verifier) {
	a := A{Content: []byte("asdf")}
	sigBytes, err := signer.SignObject(a, canonicalizer.OptionExcludeField("Signature"))
	require.NoError(t, err)

	a.Signature = sigBytes

	err = verifier.VerifyObject(sigBytes, a, canonicalizer.OptionExcludeField("Signature"))
	require.NoError(t, err)
}
