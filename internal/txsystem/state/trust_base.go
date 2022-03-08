package state

import (
	"crypto"
	"encoding/hex"
	"fmt"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

func parseTrustBase(keys []string) ([]crypto.PublicKey, error) {
	if len(keys) == 0 {
		return nil, errors.New("unicity trust base must be defined")
	}
	var publicKeys []crypto.PublicKey
	for _, hexKey := range keys {
		keyBytes, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("error decoding trust base hex hexKey: %w", err)
		}
		verifier, err := abcrypto.NewVerifierSecp256k1(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing public key: %w", err)
		}
		pubkey, err := verifier.UnmarshalPubKey()
		if err != nil {
			return nil, err
		}
		publicKeys = append(publicKeys, pubkey)
	}
	return publicKeys, nil
}
