package cmd

import (
	"crypto/rand"
	"path"

	"github.com/spf13/cobra"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"github.com/libp2p/go-libp2p-core/crypto"
)

const (
	secp256k1 = "secp256k1"

	forceKeyGenCmdFlag  = "force-key-gen"
	keyFileCmdFlag      = "key-file"
	defaultKeysFileName = "keys.json"
)

type (
	Keys struct {
		SigningPrivateKey    abcrypto.Signer
		EncryptionPrivateKey crypto.PrivKey
	}

	KeysConfig struct {
		HomeDir                     *string
		KeyFilePath                 string
		ForceKeyGeneration          bool
		defaultKeysRelativeFilePath string
	}

	keyFile struct {
		SigningPrivateKey    key `json:"signing"`
		EncryptionPrivateKey key `json:"encryption"`
	}

	key struct {
		Algorithm  string `json:"algorithm"`
		PrivateKey []byte `json:"privateKey"`
	}
)

func NewKeysConf(conf *baseConfiguration) *KeysConfig {
	return &KeysConfig{HomeDir: &conf.HomeDir}
}

func (keysConf *KeysConfig) addCmdFlags(cmd *cobra.Command, relativeDir string) {
	keysConf.defaultKeysRelativeFilePath = path.Join(relativeDir, defaultKeysFileName)
	cmd.Flags().BoolVarP(&keysConf.ForceKeyGeneration, forceKeyGenCmdFlag, "f", false, "generates new keys, overwrites existing keys file")
	fullKeysFilePath := path.Join("$AB_HOME", keysConf.defaultKeysRelativeFilePath)
	cmd.Flags().StringVarP(&keysConf.KeyFilePath, keyFileCmdFlag, "k", "", "path to the keys file (default: "+fullKeysFilePath+"). If key file does not exist and flag -f is present then new keys are generated.")
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

func (keysConf *KeysConfig) GetKeyFileLocation() string {
	if keysConf.KeyFilePath != "" {
		return keysConf.KeyFilePath
	}
	return path.Join(*keysConf.HomeDir, keysConf.defaultKeysRelativeFilePath)
}

// LoadKeys loads signing and encryption keys.
func LoadKeys(file string, forceGeneration bool) (*Keys, error) {
	if forceGeneration {
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
		return nil, errors.Errorf("keys file %s not found", file)
	}
	kf, err := util.ReadJsonFile(file, &keyFile{})
	if err != nil {
		return nil, err
	}
	if kf.SigningPrivateKey.Algorithm != secp256k1 {
		return nil, errors.Errorf("signing key algorithm %v is not supported", kf.SigningPrivateKey.Algorithm)
	}
	if kf.EncryptionPrivateKey.Algorithm != secp256k1 {
		return nil, errors.Errorf("encryption key algorithm %v is not supported", kf.EncryptionPrivateKey.Algorithm)
	}

	signingKey, err := abcrypto.NewInMemorySecp256K1SignerFromKey(kf.SigningPrivateKey.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "invalid signing key")
	}
	encryptionKey, err := crypto.UnmarshalSecp256k1PrivateKey(kf.EncryptionPrivateKey.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "invalid encryption key")
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
