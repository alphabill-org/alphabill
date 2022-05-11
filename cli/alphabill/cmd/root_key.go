package cmd

import (
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type rootKey struct {
	PrivateKey string `json:"privateKey"`
}

func newRootKey() (*rootKey, error) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	key, err := signer.MarshalPrivateKey()
	if err != nil {
		return nil, err
	}
	return &rootKey{PrivateKey: hexutil.Encode(key)}, nil
}

func (rk *rootKey) toSigner() (abcrypto.Signer, error) {
	rkBytes, err := hexutil.Decode(rk.PrivateKey)
	if err != nil {
		return nil, err
	}
	return abcrypto.NewInMemorySecp256K1SignerFromKey(rkBytes)
}
