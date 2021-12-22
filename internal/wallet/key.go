package wallet

import (
	"alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
)

type key struct {
	pubKey           []byte
	pubKeyHashSha256 []byte
	pubKeyHashSha512 []byte
	signer           crypto.Signer
	verifier         crypto.Verifier
}

func NewKey() (*key, error) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}
	return &key{
		pubKey:           pubKey,
		pubKeyHashSha256: hash.Sum256(pubKey),
		pubKeyHashSha512: hash.Sum512(pubKey),
		signer:           signer,
		verifier:         verifier,
	}, nil
}
