package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/libp2p/go-libp2p/core/peer"
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
	// Consensus params
	// Total number of root nodes
	TotalNodes uint32
	// Block rate t3 or min consensus rate for distributed root chain
	BlockRateMs uint32
	// Time to abandon proposal and vote for timeout (only used in distributed implementation)
	ConsensusTimeoutMs uint32
	// Hash algorithm for UnicityTree calculation
	HashAlgorithm string
}

// newRootGenesisCmd creates a new cobra command for the root-genesis component.
func newRootGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &rootGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, defaultRootChainDir)}
	var cmd = &cobra.Command{
		Use:   "root-genesis",
		Short: "Tools to work with root chain genesis files",
	}
	cmd.AddCommand(newGenesisCmd(config))
	cmd.AddCommand(combineRootGenesisCmd(config))
	cmd.AddCommand(signRootGenesisCmd(config))
	cmd.AddCommand(genTrustBaseCmd(config))
	cmd.AddCommand(signTrustBaseCmd(config))
	return cmd
}

func newGenesisCmd(config *rootGenesisConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "new",
		Short: "Generates root chain genesis file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootGenesisRunFunc(config)
		},
	}
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringSliceVarP(&config.PartitionNodeGenesisFiles, partitionRecordFile, "p", []string{}, "path to partition node genesis files")
	if err := cmd.MarkFlagRequired(partitionRecordFile); err != nil {
		panic(err)
	}
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")
	// Consensus params
	cmd.Flags().Uint32Var(&config.TotalNodes, "total-nodes", 1, "total number of root nodes")
	cmd.Flags().Uint32Var(&config.BlockRateMs, "block-rate", genesis.DefaultBlockRateMs, "minimal rate (in milliseconds) at which root certifies requests from partition")
	cmd.Flags().Uint32Var(&config.ConsensusTimeoutMs, "consensus-timeout", genesis.DefaultConsensusTimeout, "time (in milliseconds) until round timeout (must be at least block-rate+2000)")
	cmd.Flags().StringVar(&config.HashAlgorithm, "hash-algorithm", "SHA-256", "hash algorithm to be used")
	return cmd
}

// getOutputDir returns custom output directory if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $AB_HOME is initialized.
func (c *rootGenesisConfig) getOutputDir() string {
	var outputDir string
	if c.OutputDir != "" {
		outputDir = c.OutputDir
		// if key file path not set, then set it to the output path
		if c.Keys.KeyFilePath == "" {
			c.Keys.KeyFilePath = filepath.Join(c.OutputDir, defaultKeysFileName)
		}
	} else {
		outputDir = c.Base.defaultRootchainDir()
	}
	return outputDir
}

func rootGenesisRunFunc(config *rootGenesisConfig) error {
	// ensure output dir is present before keys generation
	dir := config.getOutputDir()
	// create directory -rwx------
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("genesis file directory create failed: %w", err)
	}
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to read root chain keys from file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	pn, err := loadPartitionNodeGenesisFiles(config.PartitionNodeGenesisFiles)
	if err != nil {
		return fmt.Errorf("failed to read partition genesis files: %w", err)
	}
	pr, err := rootgenesis.NewPartitionRecordFromNodes(pn)
	if err != nil {
		return fmt.Errorf("genesis partition record generation failed: %w", err)
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to extract root ID from public key: %w", err)
	}
	encPubKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return fmt.Errorf("root public key conversion failed: %w", err)
	}

	rg, pg, err := rootgenesis.NewRootGenesis(
		peerID.String(),
		keys.SigningPrivateKey,
		encPubKeyBytes,
		pr,
		rootgenesis.WithTotalNodes(config.TotalNodes),
		rootgenesis.WithBlockRate(config.BlockRateMs),
		rootgenesis.WithConsensusTimeout(config.ConsensusTimeoutMs),
	)
	if err != nil {
		return fmt.Errorf("generate root genesis record failed: %w", err)
	}
	if err = saveRootGenesisFile(rg, config.getOutputDir()); err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	if err = savePartitionGenesisFiles(pg, config.getOutputDir()); err != nil {
		return fmt.Errorf("save partition genesis failed: %w", err)
	}
	return nil
}

func loadPartitionNodeGenesisFiles(paths []string) ([]*genesis.PartitionNode, error) {
	var pns []*genesis.PartitionNode
	for _, p := range paths {
		pr, err := util.ReadJsonFile(p, &genesis.PartitionNode{Version: 1})
		if err != nil {
			return nil, fmt.Errorf("read partition genesis file '%s' failed: %w", p, err)
		}
		pns = append(pns, pr)
	}
	return pns, nil
}

func saveRootGenesisFile(rg *genesis.RootGenesis, outputDir string) error {
	outputFile := filepath.Join(outputDir, rootGenesisFileName)
	return util.WriteJsonFile(outputFile, rg)
}

func savePartitionGenesisFiles(pgs []*genesis.PartitionGenesis, outputDir string) error {
	for _, pg := range pgs {
		err := savePartitionGenesisFile(pg, outputDir)
		if err != nil {
			return fmt.Errorf("save partition %X genesis failed: %w", pg.PartitionDescription.PartitionID, err)
		}
	}
	return nil
}

func savePartitionGenesisFile(pg *genesis.PartitionGenesis, outputDir string) error {
	filename := fmt.Sprintf("partition-genesis-%d.json", pg.PartitionDescription.PartitionID)
	outputFile := filepath.Join(outputDir, filename)
	return util.WriteJsonFile(outputFile, pg)
}
