package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/partition"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	utGenesisFileName = "node-genesis.json"
	utDir             = "tokens"
)

type userTokenPartitionGenesisConfig struct {
	Base             *baseConfiguration
	Keys             *keysConfig
	SystemIdentifier []byte
	Output           string
	T2Timeout        uint32
}

func newUserTokenGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &userTokenPartitionGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, utDir)}
	var cmd = &cobra.Command{
		Use:   "tokens-genesis",
		Short: "Generates a genesis file for the User-Defined Token partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return utGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", tokens.DefaultSystemIdentifier, "system identifier in HEX format")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/tokens/node-genesis.json)")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	config.Keys.addCmdFlags(cmd)
	return cmd
}

func utGenesisRunFun(_ context.Context, config *userTokenPartitionGenesisConfig) error {
	utHomePath := filepath.Join(config.Base.HomeDir, utDir)
	if !util.FileExists(utHomePath) {
		err := os.MkdirAll(utHomePath, 0700) // -rwx------)
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(utHomePath)
	if util.FileExists(nodeGenesisFile) {
		return fmt.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to load keys %v: %w", config.Keys.GetKeyFileLocation(), err)
	}

	txSystem, err := tokens.NewTxSystem(
		config.Base.Logger,
		tokens.WithSystemIdentifier(config.SystemIdentifier),
		tokens.WithTrustBase(map[string]abcrypto.Verifier{"genesis": nil}),
	)
	if err != nil {
		return err
	}

	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		txSystem,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
		partition.WithT2Timeout(config.T2Timeout),
	)
	if err != nil {
		return err
	}
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *userTokenPartitionGenesisConfig) getNodeGenesisFileLocation(utHomePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(utHomePath, utGenesisFileName)
}
