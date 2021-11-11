package crypto

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain/canonicalizer"
)

type (
	// Signer common component between Block Chain Machine components for digitally signing data.
	Signer interface {
		// SignBytes signs the data using the signatureScheme and private key specified by the Signer.
		// Returns signature bytes or error.
		SignBytes(data []byte) ([]byte, error)
		// SignObject converts the object into bytes and signs the bytes. Returns signature bytes or error.
		SignObject(obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) ([]byte, error)
		// MarshalPrivateKey returns the private key bytes in ... so these could be unmarshalled later to create the Signer.
		MarshalPrivateKey() ([]byte, error)
		// Verifier returns a verifier that verifies using the public key part.
		Verifier() Verifier
	}

	// Verifier common component between BlockChain machine components for verifying signatures.
	Verifier interface {
		// VerifyBytes verifies the bytes against the signature, using the internal public key.
		VerifyBytes(sig []byte, data []byte) error
		// VerifyObject verifies that signature authenticates the object canonical form with the public key inside Verifier.
		VerifyObject(sig []byte, obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) error
		// MarshalPublicKey marshal verifier public key to bytes.
		MarshalPublicKey() ([]byte, error)
	}

	// SignatureScheme is type for defining different signing algorithms.
	SignatureScheme byte
)

// List of all signing schemes supported by the blockchain machine
const (
	ellipticP256Sha512Asn1 = SignatureScheme(iota)
	ellipticEd25519Sha512Asn1
)

const (
	DefaultSignatureScheme = ellipticEd25519Sha512Asn1
)
