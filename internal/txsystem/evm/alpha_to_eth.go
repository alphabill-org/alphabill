package evm

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

var alpha2Wei = new(big.Int).Exp(big.NewInt(10), big.NewInt(10), nil)
var alpha2WeiRoundCorrector = new(big.Int).Div(alpha2Wei, big.NewInt(2))

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
	pubKey, err := templates.ExtractPubKey(predArg)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to extract public key from fee credit owner proof, %w", err)
	}
	return generateAddress(pubKey)
}

// Smallest alphabill unit 10^-8 is called "tema"
// 1 ETH = 1 ALPHA, from that ETH/ALPHA = 1 and also 10^18 wei / 10^8 tema
// That means tema = 10^10 wei and 1 wei = 10^-10 tema

// alphaToWei - converts from alpha to wei, assuming 1:1 exchange
// 1 wei = 1 tema / 10^10
func alphaToWei(alpha uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(alpha), alpha2Wei)
}

// weiToAlpha - converts from wei to alpha, rounding half up.
// 1 wei = wei * 10^10 / 10^18
func weiToAlpha(wei *big.Int) uint64 {
	return new(big.Int).Div(new(big.Int).Add(wei, alpha2WeiRoundCorrector), alpha2Wei).Uint64()
}
