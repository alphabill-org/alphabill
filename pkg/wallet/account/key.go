package account

import (
	"crypto/ecdsa"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/hash"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	acc "github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip39"
)

type (
	Keys struct {
		Mnemonic   string
		MasterKey  *hdkeychain.ExtendedKey
		AccountKey *AccountKey
	}

	AccountKey struct {
		PubKey         []byte     `json:"pubKey"` // compressed secp256k1 key 33 bytes
		PrivKey        []byte     `json:"privKey"`
		PubKeyHash     *KeyHashes `json:"pubKeyHash"`
		DerivationPath []byte     `json:"derivationPath"`
	}

	KeyHashes struct {
		Sha256 []byte `json:"sha256"`
	}
)

const mnemonicEntropyBitSize = 128

// NewKeys generates new wallet keys from given mnemonic seed, or generates mnemonic first if empty string is provided
func NewKeys(mnemonic string) (*Keys, error) {
	if mnemonic == "" {
		var err error
		mnemonic, err = generateMnemonic()
		if err != nil {
			return nil, err
		}
	}

	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic")
	}
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}

	// only HDPrivateKeyID is used from chaincfg.MainNetParams,
	// it is used as version flag in extended key, which in turn is used to identify the extended key's type.
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	ac, err := NewAccountKey(masterKey, NewDerivationPath(0))
	if err != nil {
		return nil, err
	}
	return &Keys{
		Mnemonic:   mnemonic,
		MasterKey:  masterKey,
		AccountKey: ac,
	}, nil
}

// NewAccountKey generates new account key from given master key and derivation path
func NewAccountKey(masterKey *hdkeychain.ExtendedKey, derivationPath string) (*AccountKey, error) {
	path, err := acc.ParseDerivationPath(derivationPath)
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
	return &AccountKey{
		PubKey:         compressedPubKey,
		PrivKey:        privateKeyBytes,
		PubKeyHash:     NewKeyHash(compressedPubKey),
		DerivationPath: []byte(derivationPath),
	}, nil
}

// NewDerivationPath returns derivation path for given account index
func NewDerivationPath(accountIndex uint64) string {
	// https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
	// m / purpose' / coin_type' / account' / change / address_index
	// m - master key
	// 44' - cryptocurrencies
	// 634' - coin type, randomly chosen number from https://github.com/satoshilabs/slips/blob/master/slip-0044.md
	// 0' - account number
	// 0 - change address 0 or 1; 0 = externally used address, 1 = internal address, currently always 0
	// 0 - address index
	// we currently have an ethereum like account based model meaning 1 account = 1 address
	derivationPath := "m/44'/634'/%d'/0/0"
	return fmt.Sprintf(derivationPath, accountIndex)
}

// NewKeyHash creates sha256/sha512 hash pair from given key
func NewKeyHash(key []byte) *KeyHashes {
	return &KeyHashes{
		Sha256: hash.Sum256(key),
	}
}

func generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(mnemonicEntropyBitSize)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// derivePrivateKey derives the private accountKey of the derivation path.
func derivePrivateKey(path acc.DerivationPath, masterKey *hdkeychain.ExtendedKey) (*ecdsa.PrivateKey, error) {
	var err error
	var derivedKey = masterKey
	for _, n := range path {
		derivedKey, err = derivedKey.Derive(n)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := derivedKey.ECPrivKey()
	if err != nil {
		return nil, err
	}
	return privateKey.ToECDSA(), nil
}
