package evm

import (
	"crypto/ecdsa"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func generateAddress(pubKeyBytes []byte) (common.Address, error) {
	if pubKeyBytes == nil {
		return common.Address{}, fmt.Errorf("public key bytes is nil")
	}
	v, err := abcrypto.NewVerifierSecp256k1(pubKeyBytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("verifier from public key error, %w", err)
	}
	key, err := v.UnmarshalPubKey()
	if err != nil {
		return common.Address{}, fmt.Errorf("unmarshal public key failed, %w", err)
	}
	addr := ethcrypto.PubkeyToAddress(*key.(*ecdsa.PublicKey))
	return addr, nil
}

// FeeCreditRecordIDFromPublicKey - create fee credit id from public key. For EVM shard part is ignored for now
// as it is not meant to shard.
func FeeCreditRecordIDFromPublicKey(_, pubKey []byte) types.UnitID {
	if pubKey == nil {
		return common.Address{}.Bytes()
	}
	addr, _ := generateAddress(pubKey)
	return addr.Bytes()
}
