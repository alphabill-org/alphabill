package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/common/crypto/canonicalizer"
	"github.com/alphabill-org/alphabill/common/hash"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

var (
	ErrInvalidArgument    = errors.New("invalid nil argument")
	ErrVerificationFailed = errors.New("verification failed")
)

type (
	verifierSecp256k1 struct {
		pubKey []byte
	}
)

// CompressedSecp256K1PublicKeySize is size of public key in compressed format
const CompressedSecp256K1PublicKeySize = 33

// NewVerifierSecp256k1 creates new verifier from an existing Secp256k1 compressed public key.
func NewVerifierSecp256k1(compressedPubKey []byte) (Verifier, error) {
	if len(compressedPubKey) != CompressedSecp256K1PublicKeySize {
		return nil, fmt.Errorf("pubkey must be %d bytes long, but is %d", CompressedSecp256K1PublicKeySize, len(compressedPubKey))
	}
	x, y := secp256k1.DecompressPubkey(compressedPubKey)
	if x == nil && y == nil {
		return nil, fmt.Errorf("public key decompress faield")
	}
	pubkey := elliptic.Marshal(secp256k1.S256(), x, y)
	return &verifierSecp256k1{pubkey}, nil
}

// VerifyBytes hashes the data with SHA256 and verifies it using the public key of the verifier.
func (v *verifierSecp256k1) VerifyBytes(sig []byte, data []byte) error {
	if v == nil || v.pubKey == nil || sig == nil || data == nil {
		return ErrInvalidArgument
	}
	return v.VerifyHash(sig, hash.Sum256(data))
}

// VerifyHash verifies the hash against the signature, using the internal public key.
func (v *verifierSecp256k1) VerifyHash(sig []byte, hash []byte) error {
	if v == nil || v.pubKey == nil || sig == nil || hash == nil {
		return ErrInvalidArgument
	}
	if len(sig) == ethcrypto.SignatureLength {
		// If signature contains recovery ID, then remove it.
		sig = sig[:len(sig)-1]
	}
	if len(sig) != ethcrypto.RecoveryIDOffset {
		return fmt.Errorf("signature length is %d b (expected %d b)", len(sig), ethcrypto.RecoveryIDOffset)
	}
	if secp256k1.VerifySignature(v.pubKey, hash, sig) {
		return nil
	}
	return ErrVerificationFailed
}

// VerifyObject verifies the signature of canonicalizable object with public key.
func (v *verifierSecp256k1) VerifyObject(sig []byte, obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) error {
	data, err := canonicalizer.Canonicalize(obj, opts...)
	if err != nil {
		return fmt.Errorf("canonicalize the object failed: %w", err)
	}
	return v.VerifyBytes(sig, data)
}

// MarshalPublicKey returns compressed public key, 33 bytes
func (v *verifierSecp256k1) MarshalPublicKey() ([]byte, error) {
	pubkey, err := v.unmarshalPubKey()
	if err != nil {
		return nil, err
	}
	return secp256k1.CompressPubkey(pubkey.X, pubkey.Y), nil
}

func (v *verifierSecp256k1) UnmarshalPubKey() (crypto.PublicKey, error) {
	return v.unmarshalPubKey()
}

func (v *verifierSecp256k1) unmarshalPubKey() (*ecdsa.PublicKey, error) {
	if v == nil || v.pubKey == nil {
		return nil, ErrInvalidArgument
	}
	pubkey, err := ethcrypto.UnmarshalPubkey(v.pubKey)
	if err != nil {
		return nil, fmt.Errorf("convert public key bytes to ECDSA public key failed: %w", err)
	}
	return pubkey, nil
}
