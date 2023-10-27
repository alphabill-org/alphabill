package crypto

import (
	"crypto"

	"github.com/alphabill-org/alphabill/common/crypto/canonicalizer"
)

type (
	// Signer component for digitally signing data.
	Signer interface {
		// SignBytes signs the data using the signatureScheme and private key specified by the Signer.
		// Returns signature bytes or error.
		SignBytes(data []byte) ([]byte, error)
		// SignHash signs the hashed using the signatureScheme and private key specified by the Signer.
		// Returns signature bytes or error.
		SignHash(data []byte) ([]byte, error)
		// SignObject converts the object into bytes and signs the bytes. Returns signature bytes or error.
		SignObject(obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) ([]byte, error)
		// MarshalPrivateKey returns the private key bytes so these could be unmarshalled later to create the Signer.
		MarshalPrivateKey() ([]byte, error)
		// Verifier returns a verifier that verifies using the public key part.
		Verifier() (Verifier, error)
	}

	// Verifier component for verifying signatures.
	Verifier interface {
		// VerifyBytes verifies the bytes against the signature, using the internal public key.
		VerifyBytes(sig []byte, data []byte) error
		// VerifyHash verifies the hash against the signature, using the internal public key.
		VerifyHash(signature []byte, hash []byte) error
		// VerifyObject verifies that signature authenticates the object canonical form with the public key inside Verifier.
		VerifyObject(sig []byte, obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) error
		// MarshalPublicKey marshal verifier public key to bytes.
		MarshalPublicKey() ([]byte, error)
		// UnmarshalPubKey unmarshal verifier public key to crypto.PublicKey
		UnmarshalPubKey() (crypto.PublicKey, error)
	}
)
