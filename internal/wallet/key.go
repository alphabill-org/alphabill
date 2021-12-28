package wallet

import (
	"alphabill-wallet-sdk/internal/crypto"
	"alphabill-wallet-sdk/internal/crypto/hash"
)

type Key struct {
	PubKey           []byte `json:"pubKey"`
	PrivKey          []byte `json:"privKey"`
	PubKeyHashSha256 []byte `json:"pubKeyHashSha256"`
	PubKeyHashSha512 []byte `json:"pubKeyHashSha512"`
}

func NewKey() (*Key, error) {
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
	privKey, err := signer.MarshalPrivateKey()
	if err != nil {
		return nil, err
	}
	return &Key{
		PubKey:           pubKey,
		PrivKey:          privKey,
		PubKeyHashSha256: hash.Sum256(pubKey),
		PubKeyHashSha512: hash.Sum512(pubKey),
	}, nil
}
