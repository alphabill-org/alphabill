package cmd

import (
	"context"
	"os"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/spf13/cobra"
)

type distributedGenesisConfig struct {
	Base *baseConfiguration

	// path to output directory where genesis files will be created (default current directory)
	OutputDir string
	// paths to root genesis record json files
	RootGenesisFiles []string
}

// getOutputDir returns custom outputdir if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $AB_HOME is initialized.
func (c *distributedGenesisConfig) getOutputDir() string {
	var outputDir string
	if c.OutputDir != "" {
		outputDir = c.OutputDir
	} else {
		outputDir = c.Base.defaultRootGenesisDir()
	}
	// -rwx------
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		panic(err)
	}
	return outputDir
}

// newRootGenesisCmd creates a new cobra command for the root-genesis component.
func newDistributedRootGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &distributedGenesisConfig{Base: baseConfig}
	var cmd = &cobra.Command{
		Use:   "distributed-root-genesis",
		Short: "Combines root chain genesis files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return distributedRootGenesisRunFunc(ctx, config)
		},
	}
	cmd.Flags().StringSliceVarP(&config.RootGenesisFiles, rootGenesisFileName, "r", []string{}, "path to root node genesis files")
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")

	err := cmd.MarkFlagRequired(rootGenesisFileName)
	if err != nil {
		panic(err)
	}
	return cmd
}

func distributedRootGenesisRunFunc(_ context.Context, config *distributedGenesisConfig) error {
	// ensure output dir is present before keys generation
	_ = config.getOutputDir()
	rgs, err := loadRootGenesisFiles(config.RootGenesisFiles)
	if err != nil {
		return err
	}
	// Combine root genesis files to single distributed genesis file
	rg, pg, err := rootchain.NewDistributedRootGenesis(rgs)
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
