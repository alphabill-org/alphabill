package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/spf13/cobra"
)

type genVarFileConfig struct {
	Base *baseConfiguration

	PartitionNodeGenesisFiles []string
	ShardID                   string
	EpochNumber               uint64
	RoundNumber               uint64
	OutputDir                 string
}

func newVarCmd(baseConfig *baseConfiguration) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "var",
		Short: "Tools for working with VAR files.",
	}
	cmd.AddCommand(newGenVarFileCmd(baseConfig))
	return cmd
}

func newGenVarFileCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &genVarFileConfig{Base: baseConfig}
	var cmd = &cobra.Command{
		Use:   "generate",
		Short: "Generates a VAR file from the provided node genesis files and parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateVarFileFn(config)
		},
	}
	cmd.Flags().StringSliceVarP(&config.PartitionNodeGenesisFiles, partitionRecordFile, "p", []string{}, "path to partition node genesis files")
	if err := cmd.MarkFlagRequired(partitionRecordFile); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&config.ShardID, "shard-id", "0x80", "the shard id in hex format with 0x prefix")
	cmd.Flags().Uint64Var(&config.EpochNumber, "epoch-number", 0, "the next epoch's number")
	if err := cmd.MarkFlagRequired("epoch-number"); err != nil {
		panic(err)
	}
	cmd.Flags().Uint64Var(&config.RoundNumber, "round-number", 0, "the next epoch activation shard round number")
	if err := cmd.MarkFlagRequired("round-number"); err != nil {
		panic(err)
	}
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "",
		"path to output directory where to store the generated var file (default: $AB_HOME), "+
			"creates the directory and parent directories if the path does not exist and "+
			"overwrites the output var file if it already exists.")
	return cmd
}

func generateVarFileFn(config *genVarFileConfig) error {
	outputDir := config.getOutputDir()
	if err := os.MkdirAll(outputDir, 0700); err != nil { // -rwx------
		return fmt.Errorf("failed to create var file output directory: %w", err)
	}

	// load node genesis json files
	nodeGenesisFiles, err := loadPartitionNodeGenesisFiles(config.PartitionNodeGenesisFiles)
	if err != nil {
		return fmt.Errorf("failed to read node genesis files: %w", err)
	}

	// verify all node genesis files belong to the same network and partition
	if len(nodeGenesisFiles) == 0 {
		return errors.New("at least one node genesis file must be provided")
	}
	networkID := nodeGenesisFiles[0].PartitionDescriptionRecord.NetworkID
	partitionID := nodeGenesisFiles[0].PartitionDescriptionRecord.PartitionID
	for _, ngf := range nodeGenesisFiles {
		pdr := ngf.PartitionDescriptionRecord
		if networkID != pdr.NetworkID {
			return fmt.Errorf("node genesis files network ids do not match: %d vs %d", networkID, pdr.NetworkID)
		}
		if partitionID != pdr.PartitionID {
			return fmt.Errorf("node genesis files partition ids do not match: %d vs %d", partitionID, pdr.PartitionID)
		}
	}

	// parse the shardID
	shardID := types.ShardID{}
	if err = shardID.UnmarshalText([]byte(config.ShardID)); err != nil {
		return fmt.Errorf("failed to parse shard id: %w", err)
	}

	// build VAR
	rec := &partitions.ValidatorAssignmentRecord{
		NetworkID:   networkID,
		PartitionID: partitionID,
		ShardID:     shardID,
		EpochNumber: config.EpochNumber,
		RoundNumber: config.RoundNumber,
		Nodes:       partitions.NewVarNodesFromGenesisNodes(nodeGenesisFiles),
	}
	if err = rec.Verify(nil); err != nil {
		return fmt.Errorf("invalid VAR: %w", err)
	}

	// save VAR file
	outputFile := filepath.Join(outputDir, "var.json")
	if err = util.WriteJsonFile(outputFile, rec); err != nil {
		return fmt.Errorf("failed to save VAR file '%s': %w", outputFile, err)
	}
	return nil
}

// getOutputDir returns custom output directory if provided, otherwise returns the AB_HOME directory.
func (c *genVarFileConfig) getOutputDir() string {
	if c.OutputDir != "" {
		return c.OutputDir
	}
	return c.Base.HomeDir
}
