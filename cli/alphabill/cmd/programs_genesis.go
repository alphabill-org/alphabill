package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	programGenesisFileName = "node-genesis.json"
	programsDir            = "programs"
)

type programGenesisConfig struct {
	Base             *baseConfiguration
	Keys             *keysConfig
	SystemIdentifier []byte
	Output           string
	T2Timeout        uint32
}

// newProgramGenesisCmd creates a new cobra command for the vd genesis.
func newProgramGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &programGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, programsDir)}
	var cmd = &cobra.Command{
		Use:   "programs-genesis",
		Short: "Generates a genesis file for the programs partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return programGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", program.DefaultProgramsSystemIdentifier, "system identifier in HEX format")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/programs/node-genesis.json)")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	config.Keys.addCmdFlags(cmd)
	return cmd
}

func programGenesisRunFun(ctx context.Context, config *programGenesisConfig) error {
	homePath := filepath.Join(config.Base.HomeDir, programsDir)
	if !util.FileExists(homePath) {
		err := os.MkdirAll(homePath, 0700) // -rwx------)
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(homePath)
	if util.FileExists(nodeGenesisFile) {
		return fmt.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("load keys %v failed: %w", config.Keys.GetKeyFileLocation(), err)
	}

	txSystem, err := program.New(ctx, config.SystemIdentifier)
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

func (c *programGenesisConfig) getNodeGenesisFileLocation(vdHomePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(vdHomePath, programGenesisFileName)
}
