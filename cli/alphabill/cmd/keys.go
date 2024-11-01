package cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"
)

const (
	secp256k1 = "secp256k1"

	genKeysCmdFlag      = "gen-keys"
	forceKeyGenCmdFlag  = "force"
	keyFileCmdFlag      = "key-file"
	defaultKeysFileName = "keys.json"
)

type (
	Keys struct {
		SigningPrivateKey    abcrypto.Signer
		EncryptionPrivateKey crypto.PrivKey
	}

	keysConfig struct {
		HomeDir                     *string
		KeyFilePath                 string
		GenerateKeys                bool
		ForceGeneration             bool
		defaultKeysRelativeFilePath string
	}

	keyFile struct {
		SigningPrivateKey    key `json:"signing"`
		EncryptionPrivateKey key `json:"encryption"`
	}

	key struct {
		Algorithm  string    `json:"algorithm"`
		PrivateKey hex.Bytes `json:"privateKey"`
	}
)

func NewKeysConf(conf *baseConfiguration, relativeDir string) *keysConfig {
	return &keysConfig{HomeDir: &conf.HomeDir, defaultKeysRelativeFilePath: filepath.Join(relativeDir, defaultKeysFileName)}
}

func (keysConf *keysConfig) addCmdFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&keysConf.GenerateKeys, genKeysCmdFlag, "g", false, "generates new keys if none exist")
	cmd.Flags().BoolVarP(&keysConf.ForceGeneration, forceKeyGenCmdFlag, "f", false, "forces key generation, overwriting existing keys. Must be used with -g flag")
	fullKeysFilePath := filepath.Join("$AB_HOME", keysConf.defaultKeysRelativeFilePath)
	cmd.Flags().StringVarP(&keysConf.KeyFilePath, keyFileCmdFlag, "k", "", fmt.Sprintf("path to the keys file (default: %s). If key file does not exist and flag -g is present then new keys are generated.", fullKeysFilePath))
}

// GenerateKeys generates a new signing and encryption key.
func GenerateKeys() (*Keys, error) {
	signingKey, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	encryptionKey, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Keys{
		SigningPrivateKey:    signingKey,
		EncryptionPrivateKey: encryptionKey,
	}, nil
}

func (keysConf *keysConfig) GetKeyFileLocation() string {
	if keysConf.KeyFilePath != "" {
		return keysConf.KeyFilePath
	}
	return filepath.Join(*keysConf.HomeDir, keysConf.defaultKeysRelativeFilePath)
}

// LoadKeys loads signing and encryption keys.
func LoadKeys(file string, generateNewIfNotExist bool, overwrite bool) (*Keys, error) {
	exists := util.FileExists(file)

	if (exists && overwrite) || (!exists && generateNewIfNotExist) {
		// ensure intermediate dirs exist
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			return nil, err
		}
		generateKeys, err := GenerateKeys()
		if err != nil {
			return nil, err
		}
		err = generateKeys.WriteTo(file)
		if err != nil {
			return nil, err
		}
		return generateKeys, nil
	}

	if !util.FileExists(file) {
		return nil, fmt.Errorf("keys file %s not found", file)
	}

	kf, err := util.ReadJsonFile(file, &keyFile{})
	if err != nil {
		return nil, err
	}
	if kf.SigningPrivateKey.Algorithm != secp256k1 {
		return nil, fmt.Errorf("signing key algorithm %v is not supported", kf.SigningPrivateKey.Algorithm)
	}
	if kf.EncryptionPrivateKey.Algorithm != secp256k1 {
		return nil, fmt.Errorf("encryption key algorithm %v is not supported", kf.EncryptionPrivateKey.Algorithm)
	}

	signingKey, err := abcrypto.NewInMemorySecp256K1SignerFromKey(kf.SigningPrivateKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid signing key: %w", err)
	}
	encryptionKey, err := crypto.UnmarshalSecp256k1PrivateKey(kf.EncryptionPrivateKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key: %w", err)
	}

	return &Keys{
		SigningPrivateKey:    signingKey,
		EncryptionPrivateKey: encryptionKey,
	}, nil
}

func (k *Keys) getEncryptionKeyPair() (*network.PeerKeyPair, error) {
	private, err := k.EncryptionPrivateKey.Raw()
	if err != nil {
		return nil, err
	}
	public, err := k.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	return &network.PeerKeyPair{
		PublicKey:  public,
		PrivateKey: private,
	}, nil
}

func (k *Keys) WriteTo(file string) error {
	signingKeyBytes, err := k.SigningPrivateKey.MarshalPrivateKey()
	if err != nil {
		return err
	}
	encKeyBytes, err := k.EncryptionPrivateKey.Raw()
	if err != nil {
		return err
	}
	kf := &keyFile{
		SigningPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: signingKeyBytes,
		},
		EncryptionPrivateKey: key{
			Algorithm:  secp256k1,
			PrivateKey: encKeyBytes,
		},
	}
	return util.WriteJsonFile(file, kf)
}
