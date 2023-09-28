package main

import (
	"crypto/ecdsa"
	"encoding/hex"
	"flag"
	"log"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

/*
Encodes public key to EOA address
Example usage:
go run scripts/evm/pubkey2address.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3
*/
func main() {
	// parse command line parameters
	pubKeyHex := flag.String("pubkey", "", "public key of evm free credit owner (externally owned account)")
	flag.Parse()

	// verify command line parameters
	if *pubKeyHex == "" {
		log.Fatal("public key is required")
	}

	// process command line parameters
	pubKeyBytes, err := hexutil.Decode(*pubKeyHex)
	if err != nil {
		log.Fatal(err)
	}
	if pubKeyBytes == nil {
		log.Fatal("public key is nil")
	}
	v, err := abcrypto.NewVerifierSecp256k1(pubKeyBytes)
	if err != nil {
		log.Fatalf("verifier from public key error: %v", err)
	}
	key, err := v.UnmarshalPubKey()
	if err != nil {
		log.Fatalf("unmarshal public key failed: %v", err)
	}
	addr := ethcrypto.PubkeyToAddress(*key.(*ecdsa.PublicKey))
	log.Printf("Address: %v", hex.EncodeToString(addr.Bytes()))
}
