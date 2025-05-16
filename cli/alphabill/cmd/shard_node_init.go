package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/partition"
)

const (
	keyConfFileName       = "keys.json"
	nodeInfoFileName      = "node-info.json"
	keyAlgorithmSecp256k1 = "secp256k1"
)

type (
	keyConfFlags struct {
		KeyConfFile string
		Generate    bool
	}

	shardNodeInitFlags struct {
		*baseFlags
		keyConfFlags
	}
)

func shardNodeInitCmd(baseFlags *baseFlags) *cobra.Command {
	cmdFlags := &shardNodeInitFlags{baseFlags: baseFlags}

	var cmd = &cobra.Command{
		Use:   "init",
		Short: "Generate public node info from keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shardNodeInit(cmd.Context(), cmdFlags)
		},
	}

	cmdFlags.addKeyConfFlags(cmd, true)
	return cmd
}

func shardNodeInit(_ context.Context, flags *shardNodeInitFlags) error {
	nodeInfoPath := flags.PathWithDefault("", nodeInfoFileName)
	if util.FileExists(nodeInfoPath) {
		return fmt.Errorf("node info %q already exists", nodeInfoPath)
	} else if err := os.MkdirAll(filepath.Dir(nodeInfoPath), 0700); err != nil { // -rwx------
		return err
	}
	keyConf, err := flags.loadKeyConf(flags.baseFlags, flags.Generate)
	if err != nil {
		return err
	}
	nodeID, err := keyConf.NodeID()
	if err != nil {
		return fmt.Errorf("failed to get node identifier: %w", err)
	}
	signer, err := keyConf.Signer()
	if err != nil {
		return err
	}
	sigVerifier, err := signer.Verifier()
	if err != nil {
		return fmt.Errorf("failed to create verifier for sigKey: %w", err)
	}
	sigKey, err := sigVerifier.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("failed to marshal sigKey: %w", err)
	}
	nodeInfo := &types.NodeInfo{
		NodeID: nodeID.String(),
		SigKey: sigKey,
		Stake:  1,
	}

	return util.WriteJsonFile(nodeInfoPath, nodeInfo)
}

func (c *keyConfFlags) addKeyConfFlags(cmd *cobra.Command, enableGenerate bool) {
	if enableGenerate {
		cmd.Flags().BoolVarP(&c.Generate, "generate", "g", false, "generate a new key configuration if none exist")
	}
	cmd.Flags().StringVarP(&c.KeyConfFile, "key-conf", "k", "",
		fmt.Sprintf("path to the key configuration file (default: %s)", filepath.Join("$AB_HOME", keyConfFileName)))
}

func (c *keyConfFlags) loadKeyConf(baseFlags *baseFlags, generate bool) (ret *partition.KeyConf, err error) {
	keyConfPath := baseFlags.PathWithDefault(c.KeyConfFile, keyConfFileName)

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
			Algorithm:  keyAlgorithmSecp256k1,
			PrivateKey: sigPrivKeyBytes,
		},
		AuthKey: partition.Key{
			Algorithm:  keyAlgorithmSecp256k1,
			PrivateKey: authPrivKeyBytes,
		},
	}, nil
}
