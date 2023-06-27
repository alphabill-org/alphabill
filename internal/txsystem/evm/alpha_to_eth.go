package evm

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
)

var alpha2Wei = decimal.NewFromFloat(10).Pow(decimal.NewFromFloat(10))

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

func getAddressFromPredicateArg(predArg []byte) (common.Address, error) {
	pubKey, err := script.ExtractPubKeyFromPredicateArgument(predArg)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to extract public key from fee credit owner proof, %w", err)
	}
	return generateAddress(pubKey)
}

// 10^-8 or 10 µ alpha = 1 mia
// 1 ETH = 1 ALPHA, from that ETH/ALPHA = 1 and also 10^18 wei / 10^8 mia = 1
// That means 1 mia = 10^10 wei and 1 wei = 10^-10 mia

// alphaToWei - converts from alpha to wei, assuming 1:1 exchange
// 1 wei = 1 mia / 10^10
func alphaToWei(alpha uint64) *big.Int {
	amount := decimal.NewFromFloat(float64(alpha))
	result := amount.Mul(alpha2Wei)
	wei := new(big.Int)
	wei.SetString(result.String(), 10)
	return wei
}

// alphaToWei - converts from alpha to wei, assuming 1:1 exchange 1 "alpha" is equal to "1 eth".
// 1 wei = wei * 10^10 / 10^18
func weiToAlpha(wei *big.Int) uint64 {
	amount := decimal.RequireFromString(wei.String())
	result := amount.Div(alpha2Wei)
	f, _ := result.Float64()
	return uint64(f)
}
