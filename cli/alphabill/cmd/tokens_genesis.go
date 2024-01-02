package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	utGenesisFileName      = "node-genesis.json"
	utGenesisStateFileName = "node-genesis-state.cbor"
	utDir                  = "tokens"
)

type userTokenPartitionGenesisConfig struct {
	Base             *baseConfiguration
	Keys             *keysConfig
	SystemIdentifier []byte
	Output           string
	OutputState      string
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
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/tokens/node-genesis-state.cbor)")
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
		return fmt.Errorf("node genesis file %q already exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}
	nodeGenesisStateFile := config.getNodeGenesisStateFileLocation(utHomePath)
	if util.FileExists(nodeGenesisStateFile) {
		return fmt.Errorf("node genesis state file %q already exists", nodeGenesisStateFile)
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to load keys %v: %w", config.Keys.GetKeyFileLocation(), err)
	}

	genesisState := state.NewEmptyState()

	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
		partition.WithT2Timeout(config.T2Timeout),
	)
	if err != nil {
		return err
	}

	if err := writeStateFile(nodeGenesisStateFile, genesisState, config.SystemIdentifier); err != nil {
		return fmt.Errorf("failed to write genesis state file: %w", err)
	}

	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *userTokenPartitionGenesisConfig) getNodeGenesisFileLocation(utHomePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(utHomePath, utGenesisFileName)
}

func (c *userTokenPartitionGenesisConfig) getNodeGenesisStateFileLocation(utHomePath string) string {
	if c.OutputState != "" {
		return c.OutputState
	}
	return filepath.Join(utHomePath, utGenesisStateFileName)
}
