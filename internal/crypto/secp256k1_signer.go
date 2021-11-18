package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"io"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain/canonicalizer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

type (
	// inMemorySecp256K1Signer for using during development
	inMemorySecp256K1Signer struct {
		privKey []byte
		rand    io.Reader
	}
)

// PrivateKeySecp256K1Size is size of private in bytes
const PrivateKeySecp256K1Size = 32

// NewInMemorySecp256K1Signer generates new key pair and creates a new inMemorySecp256K1Signer.
func NewInMemorySecp256K1Signer() (*inMemorySecp256K1Signer, error) {
	_, privKey, err := generateSecp256K1KeyPair()
	if err != nil {
		return nil, err
	}
	return NewInMemoryEd25519SignerFromKeys(privKey)
}

// NewInMemoryEd25519SignerFromKeys creates signer from an existing private key.
func NewInMemoryEd25519SignerFromKeys(privKey []byte) (*inMemorySecp256K1Signer, error) {
	if len(privKey) != PrivateKeySecp256K1Size {
		return nil, errors.New(fmt.Sprintf("invalid private key length. Is %d (expected %d)", len(privKey), PrivateKeySecp256K1Size))
	}
	return &inMemorySecp256K1Signer{privKey: privKey}, nil
}

// SignBytes hashes the data with SHA256 and creates a recoverable ECDSA signature.
// The produced signature is in the 65-byte [R || S || V] format where V is 0 or 1.
func (s *inMemorySecp256K1Signer) SignBytes(data []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	return secp256k1.Sign(hash.Sum256(data), s.privKey)
}

// SignObject transforms the object to canonical form and then signs the data using SignBytes method.
func (s *inMemorySecp256K1Signer) SignObject(obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) ([]byte, error) {
	if s == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "nil argument")
	}
	data, err := canonicalizer.Canonicalize(obj, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to canonicalize the object")
	}
	return s.SignBytes(data)
}

func (s *inMemorySecp256K1Signer) Verifier() (Verifier, error) {
	ecdsaPrivKey, err := crypto.ToECDSA(s.privKey)
	if err != nil {
		return nil, err
	}
	compressPubkey := secp256k1.CompressPubkey(ecdsaPrivKey.PublicKey.X, ecdsaPrivKey.PublicKey.Y)
	return NewVerifierSecp256k1(compressPubkey)
}

func (s *inMemorySecp256K1Signer) MarshalPrivateKey() ([]byte, error) {
	return s.privKey, nil
}

func generateSecp256K1KeyPair() (pubkey, privkey []byte, err error) {
	key, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate random key")
	}
	pubkey = elliptic.Marshal(secp256k1.S256(), key.X, key.Y)

	privkey = make([]byte, 32)
	blob := key.D.Bytes()
	copy(privkey[32-len(blob):], blob)

	return pubkey, privkey, nil
}
