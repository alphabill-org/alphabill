package cmd

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/spf13/cobra"
)

const (
	partitionRecordFile = "partition-node-genesis-file"
	rootGenesisFileName = "root-genesis.json"
)

type rootGenesisConfig struct {
	Base *baseConfiguration

	// paths to partition record json files
	PartitionNodeGenesisFiles []string

	Keys *keysConfig

	// path to output directory where genesis files will be created (default current directory)
	OutputDir          string
	ForceKeyGeneration bool
}

// newRootGenesisCmd creates a new cobra command for the root-genesis component.
func newRootGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &rootGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig)}
	var cmd = &cobra.Command{
		Use:   "root-genesis",
		Short: "Generates root chain genesis files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootGenesisRunFunc(ctx, config)
		},
	}
	config.Keys.addCmdFlags(cmd, defaultRootChainDir)
	cmd.Flags().StringSliceVarP(&config.PartitionNodeGenesisFiles, partitionRecordFile, "p", []string{}, "path to partition node genesis files")
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")

	err := cmd.MarkFlagRequired(partitionRecordFile)
	if err != nil {
		panic(err)
	}
	return cmd
}

// getOutputDir returns custom outputdir if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $AB_HOME is initialized.
func (c *rootGenesisConfig) getOutputDir() string {
	if c.OutputDir != "" {
		return c.OutputDir
	}
	defaultOutputDir := c.Base.defaultRootGenesisDir()
	err := os.MkdirAll(defaultOutputDir, 0700) // -rwx------
	if err != nil {
		panic(err)
	}
	return defaultOutputDir
}

func rootGenesisRunFunc(_ context.Context, config *rootGenesisConfig) error {
	// ensure output dir is present before keys generation
	_ = config.getOutputDir()
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to read root chain keys from file '%s'", config.Keys.GetKeyFileLocation())
	}

	pr, err := loadPartitionNodeGenesisFiles(config.PartitionNodeGenesisFiles)
	if err != nil {
		return err
	}
	privateKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}

	encVerifier, err := crypto.NewVerifierSecp256k1(privateKeyBytes)
	if err != nil {
		return err
	}

	rg, pg, err := rootchain.NewGenesisFromPartitionNodes(pr, 2500, keys.SigningPrivateKey, encVerifier)
	if err != nil {
		return err
	}
	err = saveRootGenesisFile(rg, config.getOutputDir())
	if err != nil {
		return err
	}
	err = savePartitionGenesisFiles(pg, config.getOutputDir())
	if err != nil {
		return err
	}
	return nil
}

func loadPartitionNodeGenesisFiles(paths []string) ([]*genesis.PartitionNode, error) {
	var pns []*genesis.PartitionNode
	for _, p := range paths {
		pr, err := util.ReadJsonFile(p, &genesis.PartitionNode{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read partition node genesis file '%s'", p)
		}
		pns = append(pns, pr)
	}
	return pns, nil
}

func saveRootGenesisFile(rg *genesis.RootGenesis, outputDir string) error {
	outputFile := path.Join(outputDir, rootGenesisFileName)
	return util.WriteJsonFile(outputFile, rg)
}

func savePartitionGenesisFiles(pgs []*genesis.PartitionGenesis, outputDir string) error {
	for _, pg := range pgs {
		err := savePartitionGenesisFile(pg, outputDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func savePartitionGenesisFile(pg *genesis.PartitionGenesis, outputDir string) error {
	sid := binary.BigEndian.Uint32(pg.SystemDescriptionRecord.SystemIdentifier)
	filename := fmt.Sprintf("partition-genesis-%d.json", sid)
	outputFile := path.Join(outputDir, filename)
	return util.WriteJsonFile(outputFile, pg)
}
