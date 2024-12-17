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
		SignPrivKey abcrypto.Signer
		AuthPrivKey crypto.PrivKey
	}

	keysConfig struct {
		HomeDir                     *string
		KeyFilePath                 string
		GenerateKeys                bool
		ForceGeneration             bool
		defaultKeysRelativeFilePath string
	}

	keyFile struct {
		SignKey key `json:"signKey"`
		AuthKey key `json:"authKey"`
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

// GenerateKeys generates a new signing and authentication key.
func GenerateKeys() (*Keys, error) {
	signKey, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	authPrivKey, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Keys{
		SignPrivKey: signKey,
		AuthPrivKey: authPrivKey,
	}, nil
}

func (keysConf *keysConfig) GetKeyFileLocation() string {
	if keysConf.KeyFilePath != "" {
		return keysConf.KeyFilePath
	}
	return filepath.Join(*keysConf.HomeDir, keysConf.defaultKeysRelativeFilePath)
}

// LoadKeys loads signing and authentication keys.
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
	if kf.SignKey.Algorithm != secp256k1 {
		return nil, fmt.Errorf("signing key algorithm %v is not supported", kf.SignKey.Algorithm)
	}
	if kf.AuthKey.Algorithm != secp256k1 {
		return nil, fmt.Errorf("authentication key algorithm %v is not supported", kf.AuthKey.Algorithm)
	}

	signPrivKey, err := abcrypto.NewInMemorySecp256K1SignerFromKey(kf.SignKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid signing key: %w", err)
	}
	authPrivKey, err := crypto.UnmarshalSecp256k1PrivateKey(kf.AuthKey.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid authentication key: %w", err)
	}

	return &Keys{
		SignPrivKey: signPrivKey,
		AuthPrivKey: authPrivKey,
	}, nil
}

func (k *Keys) getAuthKeyPair() (*network.PeerKeyPair, error) {
	private, err := k.AuthPrivKey.Raw()
	if err != nil {
		return nil, err
	}
	public, err := k.AuthPrivKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	return &network.PeerKeyPair{
		PublicKey:  public,
		PrivateKey: private,
	}, nil
}

func (k *Keys) WriteTo(file string) error {
	signPrivKeyBytes, err := k.SignPrivKey.MarshalPrivateKey()
	if err != nil {
		return err
	}
	authPrivKeyBytes, err := k.AuthPrivKey.Raw()
	if err != nil {
		return err
	}
	kf := &keyFile{
		SignKey: key{
			Algorithm:  secp256k1,
			PrivateKey: signPrivKeyBytes,
		},
		AuthKey: key{
			Algorithm:  secp256k1,
			PrivateKey: authPrivKeyBytes,
		},
	}
	return util.WriteJsonFile(file, kf)
}
