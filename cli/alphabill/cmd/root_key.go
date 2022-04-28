package cmd

import (
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/libp2p/go-libp2p-core/crypto"
)

type rootKey struct {
	PrivateKey string `json:"privateKey"`
}

func (rk *rootKey) toSigner() (abcrypto.Signer, error) {
	rkBytes, err := hexutil.Decode(rk.PrivateKey)
	if err != nil {
		return nil, err
	}
	return abcrypto.NewInMemorySecp256K1SignerFromKey(rkBytes)
}

func (rk *rootKey) toPeerKeyPair() (*network.PeerKeyPair, error) {
	signer, err := rk.toSigner()
	if err != nil {
		return nil, err
	}
	privateKey, err := signer.MarshalPrivateKey()
	if err != nil {
		return nil, err
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	publicKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	privKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	pubKey, err := crypto.UnmarshalSecp256k1PublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	return &network.PeerKeyPair{
		PublicKey:  pubKeyBytes,
		PrivateKey: privKeyBytes,
	}, nil
}
