package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto/canonicalizer"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

type (
	// InMemorySecp256K1Signer for using during development
	InMemorySecp256K1Signer struct {
		privKey []byte
	}
)

// PrivateKeySecp256K1Size is the size of the private key in bytes
const PrivateKeySecp256K1Size = 32

var errSignerNil = errors.New("signer is nil")

// NewInMemorySecp256K1Signer generates new key pair and creates a new InMemorySecp256K1Signer.
func NewInMemorySecp256K1Signer() (*InMemorySecp256K1Signer, error) {
	privKey, err := generateSecp256K1PrivateKey()
	if err != nil {
		return nil, err
	}
	return NewInMemorySecp256K1SignerFromKey(privKey)
}

// NewInMemorySecp256K1SignerFromKey creates signer from an existing private key.
func NewInMemorySecp256K1SignerFromKey(privKey []byte) (*InMemorySecp256K1Signer, error) {
	if len(privKey) != PrivateKeySecp256K1Size {
		return nil, fmt.Errorf("invalid private key length. Is %d (expected %d)", len(privKey), PrivateKeySecp256K1Size)
	}
	return &InMemorySecp256K1Signer{privKey: privKey}, nil
}

// SignBytes hashes the data with SHA256 and creates a recoverable ECDSA signature.
// The produced signature is in the 65-byte [R || S || V] format where V is 0 or 1.
func (s *InMemorySecp256K1Signer) SignBytes(data []byte) ([]byte, error) {
	if s == nil {
		return nil, errSignerNil
	}
	if data == nil {
		return nil, fmt.Errorf("data is nil")
	}
	return s.SignHash(hash.Sum256(data))
}

// SignHash creates a recoverable ECDSA signature.
// The produced signature is in the 65-byte [R || S || V] format where V is 0 or 1.
func (s *InMemorySecp256K1Signer) SignHash(hash []byte) ([]byte, error) {
	if s == nil {
		return nil, errSignerNil
	}
	if hash == nil {
		return nil, fmt.Errorf("hash is nil")
	}
	return secp256k1.Sign(hash, s.privKey)
}

// SignObject transforms the object to canonical form and then signs the data using SignBytes method.
func (s *InMemorySecp256K1Signer) SignObject(obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) ([]byte, error) {
	if s == nil {
		return nil, errSignerNil
	}
	data, err := canonicalizer.Canonicalize(obj, opts...)
	if err != nil {
		return nil, fmt.Errorf("canonicalize the object failed: %w", err)
	}
	return s.SignBytes(data)
}

func (s *InMemorySecp256K1Signer) Verifier() (Verifier, error) {
	ecdsaPrivKey, err := crypto.ToECDSA(s.privKey)
	if err != nil {
		return nil, err
	}
	compressPubkey := secp256k1.CompressPubkey(ecdsaPrivKey.PublicKey.X, ecdsaPrivKey.PublicKey.Y)
	return NewVerifierSecp256k1(compressPubkey)
}

func (s *InMemorySecp256K1Signer) MarshalPrivateKey() ([]byte, error) {
	return s.privKey, nil
}

func generateSecp256K1PrivateKey() (privkey []byte, err error) {
	key, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("random key generation failed: %w", err)
	}

	privkey = make([]byte, 32)
	blob := key.D.Bytes()
	copy(privkey[32-len(blob):], blob)

	return privkey, nil
}
