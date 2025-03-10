package cmd

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"
)

const (
	secp256k1       = "secp256k1"
	keyConfFileName = "keys.json"
)

type (
	keyConfFlags struct {
		KeyConfFile string
		Generate    bool
	}
)

func (c *keyConfFlags) addKeyConfFlags(cmd *cobra.Command, enableGenerate bool) {
	if enableGenerate {
		cmd.Flags().BoolVarP(&c.Generate, "generate", "g", false, "generate a new key configuration if none exist")
	}
	cmd.Flags().StringVarP(&c.KeyConfFile, "key-conf", "k", "",
		fmt.Sprintf("path to the key configuration file (default: %s)", filepath.Join("$AB_HOME", keyConfFileName)))
}

func (c *keyConfFlags) loadKeyConf(baseFlags *baseFlags, generate bool) (ret *partition.KeyConf, err error) {
	keyConfPath := baseFlags.pathWithDefault(c.KeyConfFile, keyConfFileName)

	if generate && !util.FileExists(keyConfPath) {
		// ensure intermediate dirs exist
		if err := os.MkdirAll(filepath.Dir(keyConfPath), 0700); err != nil {
			return nil, err
		}
		keyConf, err := generateKeys()
		if err != nil {
			return nil, err
		}
		if err := util.WriteJsonFile(keyConfPath, keyConf); err != nil {
			return nil, err
		}
		return keyConf, nil
	}

	return ret, baseFlags.loadConf(keyConfPath, keyConfFileName, &ret)
}

func generateKeys() (*partition.KeyConf, error) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		return nil, err
	}
	sigPrivKeyBytes, err := signer.MarshalPrivateKey()
	if err != nil {
		return nil, err
	}
	authPrivKey, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	authPrivKeyBytes, err := authPrivKey.Raw()
	if err != nil {
		return nil, err
	}
	return &partition.KeyConf{
		SigKey: partition.Key{
			Algorithm:  secp256k1,
			PrivateKey: sigPrivKeyBytes,
		},
		AuthKey: partition.Key{
			Algorithm:  secp256k1,
			PrivateKey: authPrivKeyBytes,
		},
	}, nil
}
