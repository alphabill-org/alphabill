package wallet

import (
	"crypto/ecdsa"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

type accountKey struct {
	PubKey           []byte `json:"pubKey"` // compressed secp256k1 key 33 bytes
	PrivKey          []byte `json:"privKey"`
	PubKeyHashSha256 []byte `json:"pubKeyHashSha256"`
	PubKeyHashSha512 []byte `json:"pubKeyHashSha512"`
	DerivationPath   []byte `json:"derivationPath"`
}

func newAccountKey(masterKey *hdkeychain.ExtendedKey, derivationPath string) (*accountKey, error) {
	path, err := accounts.ParseDerivationPath(derivationPath)
	if err != nil {
		return nil, err
	}

	privateKey, err := derivePrivateKey(path, masterKey)
	if err != nil {
		return nil, err
	}
	privateKeyBytes := crypto.FromECDSA(privateKey)

	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	compressedPubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}
	return &accountKey{
		PubKey:           compressedPubKey,
		PrivKey:          privateKeyBytes,
		PubKeyHashSha256: hash.Sum256(compressedPubKey),
		PubKeyHashSha512: hash.Sum512(compressedPubKey),
		DerivationPath:   []byte(derivationPath),
	}, nil
}

// derivePrivateKey derives the private accountKey of the derivation path.
func derivePrivateKey(path accounts.DerivationPath, masterKey *hdkeychain.ExtendedKey) (*ecdsa.PrivateKey, error) {
	var err error
	var derivedKey = masterKey
	for _, n := range path {
		derivedKey, err = derivedKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := masterKey.ECPrivKey()
	if err != nil {
		return nil, err
	}
	return privateKey.ToECDSA(), nil
}
