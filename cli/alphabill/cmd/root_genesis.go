package cmd

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
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
	// Optionally define a different, higher quorum threshold
	QuorumThreshold uint32
	// Hash algorithm for UnicityTree calculation
	HashAlgorithm string
}

// newRootGenesisCmd creates a new cobra command for the root-genesis component.
// there will be other commands added in the future
func newRootGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &rootGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, defaultRootChainDir)}
	var cmd = &cobra.Command{
		Use:   "root-genesis",
		Short: "Generates root chain genesis files",
	}
	cmd.AddCommand(newGenesisCmd(config))
	cmd.AddCommand(combineRootGenesisCmd(config))
	cmd.AddCommand(signRootGenesisCmd(config))
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
	cmd.Flags().Uint32Var(&config.BlockRateMs, "block-rate", genesis.DefaultBlockRateMs, "Unicity Certificate rate")
	cmd.Flags().Uint32Var(&config.ConsensusTimeoutMs, "consensus-timeout", genesis.DefaultConsensusTimeout, "time (in milliseconds) until round timeout, must be at least block-rate+2000 (only DRC)")
	cmd.Flags().Uint32Var(&config.QuorumThreshold, "quorum-threshold", 0, "define higher quorum threshold instead of calculated default")
	cmd.Flags().StringVar(&config.HashAlgorithm, "hash-algorithm", "SHA-256", "Hash algorithm to be used")
	return cmd
}

// getOutputDir returns custom outputdir if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
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
		outputDir = c.Base.defaultRootGenesisDir()
	}
	return outputDir
}

func (c *rootGenesisConfig) getQuorumThreshold() uint32 {
	// If not set by user, calculate minimal threshold
	if c.QuorumThreshold == 0 {
		return genesis.GetMinQuorumThreshold(c.TotalNodes)
	}
	return c.QuorumThreshold
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
		return fmt.Errorf("failed to extract root ID from publick key: %w", err)
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
		rootgenesis.WithQuorumThreshold(config.getQuorumThreshold()),
		rootgenesis.WithBlockRate(config.BlockRateMs),
		rootgenesis.WithConsensusTimeout(config.ConsensusTimeoutMs))
	if err != nil {
		return fmt.Errorf("generate root genesis record failed: %w", err)
	}
	err = saveRootGenesisFile(rg, config.getOutputDir())
	if err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	err = savePartitionGenesisFiles(pg, config.getOutputDir())
	if err != nil {
		return fmt.Errorf("save partition genesis failed: %w", err)
	}
	return nil
}

func loadPartitionNodeGenesisFiles(paths []string) ([]*genesis.PartitionNode, error) {
	var pns []*genesis.PartitionNode
	for _, p := range paths {
		pr, err := util.ReadJsonFile(p, &genesis.PartitionNode{})
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
			return fmt.Errorf("save partition %X genesis failed: %w", pg.SystemDescriptionRecord.SystemIdentifier, err)
		}
	}
	return nil
}

func savePartitionGenesisFile(pg *genesis.PartitionGenesis, outputDir string) error {
	sid := binary.BigEndian.Uint32(pg.SystemDescriptionRecord.SystemIdentifier)
	filename := fmt.Sprintf("partition-genesis-%d.json", sid)
	outputFile := filepath.Join(outputDir, filename)
	return util.WriteJsonFile(outputFile, pg)
}
