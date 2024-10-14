package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
)

const (
	utGenesisFileName      = "node-genesis.json"
	utGenesisStateFileName = "node-genesis-state.cbor"
	utDir                  = "tokens"
)

type userTokenPartitionGenesisConfig struct {
	Base                *baseConfiguration
	Keys                *keysConfig
	PDRFilename         string
	Output              string
	OutputState         string
	AdminOwnerPredicate []byte
	FeelessMode         bool
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

	cmd.Flags().StringVar(&config.PDRFilename, "partition-description", "", "filename (full path) from where to read the Partition Description Record")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/tokens/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/tokens/node-genesis-state.cbor)")
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().BytesHexVar(&config.AdminOwnerPredicate, "admin-owner-predicate", nil, "the admin owner predicate for permissioned mode")
	cmd.Flags().BoolVar(&config.FeelessMode, "feeless-mode", false, "if true then fees are not charged, if false then fees are charged as normal; applies only for permissioned mode")
	_ = cmd.MarkFlagRequired("partition-description")
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

	pdr, err := util.ReadJsonFile(config.PDRFilename, &types.PartitionDescriptionRecord{})
	if err != nil {
		return fmt.Errorf("loading partition description: %w", err)
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

	params, err := config.getPartitionParams()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		*pdr,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithParams(params),
	)
	if err != nil {
		return err
	}

	if err := writeStateFile(nodeGenesisStateFile, genesisState); err != nil {
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

func (c *userTokenPartitionGenesisConfig) getPartitionParams() ([]byte, error) {
	src := &genesis.TokensPartitionParams{
		AdminOwnerPredicate: c.AdminOwnerPredicate,
		FeelessMode:         c.FeelessMode,
	}
	res, err := types.Cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tokens partition params: %w", err)
	}
	return res, nil
}
