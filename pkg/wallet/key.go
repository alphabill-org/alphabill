package wallet

import (
	"crypto/ecdsa"
	"errors"

	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip39"
)

type keys struct {
	mnemonic   string
	masterKey  *hdkeychain.ExtendedKey
	accountKey *accountKey
}

type accountKey struct {
	PubKey           []byte `json:"pubKey"` // compressed secp256k1 key 33 bytes
	PrivKey          []byte `json:"privKey"`
	PubKeyHashSha256 []byte `json:"pubKeyHashSha256"`
	PubKeyHashSha512 []byte `json:"pubKeyHashSha512"`
	DerivationPath   []byte `json:"derivationPath"`
}

func generateKeys(mnemonic string) (*keys, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic")
	}
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, "")
	if err != nil {
		return nil, err
	}

	// https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
	// m / purpose' / coin_type' / account' / change / address_index
	// m - master key
	// 44' - cryptocurrencies
	// 634' - coin type, randomly chosen number from https://github.com/satoshilabs/slips/blob/master/slip-0044.md
	// 0' - account number (currently use only one account)
	// 0 - change address 0 or 1; 0 = externally used address, 1 = internal address, currently always 0
	// 0 - address index
	// we currently have an ethereum like account based model meaning 1 account = 1 address and no plans to support multiple accounts at this time,
	// so we use wallet's "HD" part only for generating single key from seed
	derivationPath := "m/44'/634'/0'/0/0"

	// only HDPrivateKeyID is used from chaincfg.MainNetParams,
	// it is used as version flag in extended key, which in turn is used to identify the extended key's type.
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	ac, err := newAccountKey(masterKey, derivationPath)
	if err != nil {
		return nil, err
	}
	return &keys{
		mnemonic:   mnemonic,
		masterKey:  masterKey,
		accountKey: ac,
	}, nil
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
