package state

import (
	"crypto"
	"encoding/hex"
	"fmt"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

type trustBase struct {
	keys []crypto.PublicKey
}

func newTrustBase(hexKeys []string) (*trustBase, error) {
	if len(hexKeys) == 0 {
		return nil, errors.New("unicity trust base must be defined")
	}
	var keys []crypto.PublicKey
	for _, hexKey := range hexKeys {
		keyBytes, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("error decoding trust base hex key: %w", err)
		}
		verifier, err := abcrypto.NewVerifierSecp256k1(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing public key: %w", err)
		}
		pubkey, err := verifier.UnmarshalPubKey()
		if err != nil {
			return nil, err
		}
		keys = append(keys, pubkey)
	}
	return &trustBase{keys: keys}, nil
}
