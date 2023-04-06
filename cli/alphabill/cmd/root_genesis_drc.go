package cmd

import (
	"fmt"
	"os"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
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
	cmd.Flags().StringSliceVarP(&combineCfg.RootGenesisFiles, rootGenesisFileName, "r", []string{}, "path to root node genesis files")
	cmd.Flags().StringVarP(&combineCfg.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")

	err := cmd.MarkFlagRequired(rootGenesisFileName)
	if err != nil {
		panic(err)
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
		return fmt.Errorf("combine faild %w", err)
	}
	rgs, err := loadRootGenesisFiles(config.RootGenesisFiles)
	if err != nil {
		return err
	}
	// Combine root genesis files to single distributed genesis file
	rg, pg, err := rootgenesis.MergeRootGenesisFiles(rgs)
	if err != nil {
		return err
	}
	err = saveRootGenesisFile(rg, outputDir)
	if err != nil {
		return err
	}
	err = savePartitionGenesisFiles(pg, outputDir)
	if err != nil {
		return err
	}
	return nil
}

func loadRootGenesisFiles(paths []string) ([]*genesis.RootGenesis, error) {
	var rgs []*genesis.RootGenesis
	for _, p := range paths {
		rg, err := util.ReadJsonFile(p, &genesis.RootGenesis{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read partition node genesis file '%s'", p)
		}
		rgs = append(rgs, rg)
	}
	return rgs, nil
}

// combineRootGenesisCmd creates a new cobra command for the root-genesis component.
func signRootGenesisCmd(config *rootGenesisConfig) *cobra.Command {
	signCfg := &signGenesisConfig{Base: config.Base, Keys: NewKeysConf(config.Base, defaultRootChainDir)}
	var cmd = &cobra.Command{
		Use:   "sign",
		Short: "Sign root chain genesis file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return signRootGenesisRunFunc(signCfg)
		},
	}
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVarP(&signCfg.RootGenesisFile, rootGenesisFileName, "r", "", "path to root node genesis file")
	cmd.Flags().StringVarP(&signCfg.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")

	err := cmd.MarkFlagRequired(rootGenesisFileName)
	if err != nil {
		panic(err)
	}
	return cmd
}

func signRootGenesisRunFunc(config *signGenesisConfig) error {
	// ensure output dir is present before keys generation
	outputDir := config.Base.defaultRootGenesisDir()
	// cmd override
	if config.OutputDir != "" {
		outputDir = config.OutputDir
	}
	if err := createOutputDir(outputDir); err != nil {
		return fmt.Errorf("sign faild %w", err)
	}
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to read root chain keys from file '%s', %w", config.Keys.GetKeyFileLocation(), err)
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to extract peer id from key file %s, error %w", config.Keys.GetKeyFileLocation(), err)
	}
	encPubKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return fmt.Errorf("failed to extract encryption public key from key file %s, error %w", config.Keys.GetKeyFileLocation(), err)
	}
	rg, err := util.ReadJsonFile(config.RootGenesisFile, &genesis.RootGenesis{})
	if err != nil {
		return fmt.Errorf("root genesis file %s read error %w", config.RootGenesisFile, err)
	}
	// Combine root genesis files to single distributed genesis file
	rg, err = rootgenesis.RootGenesisAddSignature(rg, peerID.String(), keys.SigningPrivateKey, encPubKeyBytes)
	if err != nil {
		return fmt.Errorf("root genesis add signature error, %w", err)
	}
	err = saveRootGenesisFile(rg, outputDir)
	if err != nil {
		return fmt.Errorf("root genesis save error %w", err)
	}
	return nil
}
