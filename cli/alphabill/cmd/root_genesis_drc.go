package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	rootGenesisCmdName = "root-genesis"
)

type combineGenesisConfig struct {
	Base *baseConfiguration

	// path to output directory where genesis files will be created (default current directory)
	OutputDir string
	// paths to root genesis record json files
	RootGenesisFiles []string
}

type signGenesisConfig struct {
	Base *baseConfiguration

	Keys *keysConfig
	// path to output directory where genesis files will be created (default current directory)
	OutputDir string
	// paths to root genesis record json files
	RootGenesisFile string
}

// getOutputDir returns custom outputdir if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $AB_HOME is initialized.
func createOutputDir(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return fmt.Errorf("unabale to create directory, %w", err)
	}
	return nil
}

// combineRootGenesisCmd creates a new cobra command for the root-genesis component.
func combineRootGenesisCmd(config *rootGenesisConfig) *cobra.Command {
	combineCfg := &combineGenesisConfig{Base: config.Base}
	var cmd = &cobra.Command{
		Use:   "combine",
		Short: "Combines root chain genesis files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return combineRootGenesisRunFunc(combineCfg)
		},
	}
	cmd.Flags().StringVarP(&combineCfg.OutputDir, "output", "o", "", "path to output directory (default: $AB_HOME/rootchain)")
	cmd.Flags().StringSliceVar(&combineCfg.RootGenesisFiles, rootGenesisCmdName, []string{}, "path to root node genesis files")
	if err := cmd.MarkFlagRequired(rootGenesisCmdName); err != nil {
		return nil
	}
	return cmd
}

func combineRootGenesisRunFunc(config *combineGenesisConfig) error {
	// ensure output dir is present before keys generation
	outputDir := config.Base.defaultRootGenesisDir()
	// cmd override
	if config.OutputDir != "" {
		outputDir = config.OutputDir
	}
	if err := createOutputDir(outputDir); err != nil {
		return fmt.Errorf("create dir '%s' failed: %w", outputDir, err)
	}
	rgs, err := loadRootGenesisFiles(config.RootGenesisFiles)
	if err != nil {
		return fmt.Errorf("failed to read root geresis files: %w", err)
	}
	// Combine root genesis files to single distributed genesis file
	rg, pg, err := rootgenesis.MergeRootGenesisFiles(rgs)
	if err != nil {
		return fmt.Errorf("root genesis merge failed: %w", err)
	}
	err = saveRootGenesisFile(rg, outputDir)
	if err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	err = savePartitionGenesisFiles(pg, outputDir)
	if err != nil {
		return fmt.Errorf("partition genesis file save failed: %w", err)
	}
	return nil
}

func loadRootGenesisFiles(paths []string) ([]*genesis.RootGenesis, error) {
	var rgs []*genesis.RootGenesis
	for _, p := range paths {
		rg, err := util.ReadJsonFile(p, &genesis.RootGenesis{})
		if err != nil {
			return nil, fmt.Errorf("file '%s' read error: %w", p, err)
		}
		rgs = append(rgs, rg)
	}
	return rgs, nil
}

// combineRootGenesisCmd creates a new cobra command for the root-genesis component.
func signRootGenesisCmd(config *rootGenesisConfig) *cobra.Command {
	signCfg := &signGenesisConfig{Base: config.Base, Keys: config.Keys}
	var cmd = &cobra.Command{
		Use:   "sign",
		Short: "Sign root chain genesis file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return signRootGenesisRunFunc(signCfg)
		},
	}
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVarP(&signCfg.OutputDir, "output", "o", "", "path to output directory (default: $AB_HOME/rootchain)")
	cmd.Flags().StringVar(&signCfg.RootGenesisFile, rootGenesisCmdName, "", "path to root node genesis file")
	if err := cmd.MarkFlagRequired(rootGenesisCmdName); err != nil {
		return nil
	}
	return cmd
}

func signRootGenesisRunFunc(config *signGenesisConfig) error {
	// ensure output dir is present before keys generation
	outputDir := config.Base.defaultRootGenesisDir()
	// cmd override2
	if config.OutputDir != "" {
		outputDir = config.OutputDir
		// if instructed to generate keys and key path not set, then set to outputpath
		if config.Keys.GenerateKeys && config.Keys.KeyFilePath == "" {
			config.Keys.KeyFilePath = filepath.Join(outputDir, defaultKeysFileName)
		}
	}
	if err := createOutputDir(outputDir); err != nil {
		return fmt.Errorf("create dir '%s' failed: %w", outputDir, err)
	}
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to read root chain keys from file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to extract peer id from key file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	encPubKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return fmt.Errorf("failed to extract encryption public key from key file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	rg, err := util.ReadJsonFile(config.RootGenesisFile, &genesis.RootGenesis{})
	if err != nil {
		return fmt.Errorf("root genesis file '%s' read error: %w", config.RootGenesisFile, err)
	}
	// Combine root genesis files to single distributed genesis file
	rg, err = rootgenesis.RootGenesisAddSignature(rg, peerID.String(), keys.SigningPrivateKey, encPubKeyBytes)
	if err != nil {
		return fmt.Errorf("root genesis add signature failed: %w", err)
	}
	err = saveRootGenesisFile(rg, outputDir)
	if err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	return nil
}
