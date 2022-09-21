package cmd

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain"
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
func newRootGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &rootGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, defaultRootChainDir)}
	var cmd = &cobra.Command{
		Use:   "root-genesis",
		Short: "Generates root chain genesis files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootGenesisRunFunc(ctx, config)
		},
	}
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringSliceVarP(&config.PartitionNodeGenesisFiles, partitionRecordFile, "p", []string{}, "path to partition node genesis files")
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "", "path to output directory (default: $AB_HOME/rootchain)")
	// Consensus params
	cmd.Flags().Uint32Var(&config.TotalNodes, "total-nodes", 1, "total number of root nodes")
	cmd.Flags().Uint32Var(&config.BlockRateMs, "block-rate", 900, "minimal UC rate")
	cmd.Flags().Uint32Var(&config.ConsensusTimeoutMs, "consensus-timeout", 10000, "time to vote for timeout in round (only distributed root chain)")
	cmd.Flags().Uint32Var(&config.QuorumThreshold, "quorum-threshold", 0, "define higher quorum threshold instead of calculated default")
	//	cmd.Flags().StringVar(&config.HashAlgorithm, "hash-algorithm", "SHA-256", "Hash algorithm to be used")

	err := cmd.MarkFlagRequired(partitionRecordFile)
	if err != nil {
		panic(err)
	}
	return cmd
}

// getOutputDir returns custom outputdir if provided, otherwise $AB_HOME/rootchain, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $AB_HOME is initialized.
func (c *rootGenesisConfig) getOutputDir() string {
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

func (c *rootGenesisConfig) getConsensusTimeout() uint32 {
	// Only used when distributed genesis is created
	if c.TotalNodes > 1 {
		return c.ConsensusTimeoutMs
	} else {
		return 0
	}
}

func (c *rootGenesisConfig) getQuorumThreshold() uint32 {
	// Only used when distributed genesis is created
	if c.TotalNodes == 1 {
		return 0
	}
	return c.QuorumThreshold
}

func rootGenesisRunFunc(_ context.Context, config *rootGenesisConfig) error {
	// ensure output dir is present before keys generation
	_ = config.getOutputDir()
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to read root chain keys from file '%s'", config.Keys.GetKeyFileLocation())
	}
	pn, err := loadPartitionNodeGenesisFiles(config.PartitionNodeGenesisFiles)
	if err != nil {
		return err
	}
	pr, err := rootchain.NewPartitionRecordFromNodes(pn)
	if err != nil {
		return err
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encPubKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}

	rg, pg, err := rootchain.NewRootGenesis(peerID.String(), keys.SigningPrivateKey, encPubKeyBytes, pr,
		rootchain.WithTotalNodes(config.TotalNodes),
		rootchain.WithBlockRate(config.BlockRateMs),
		rootchain.WithConsensusTimeout(config.ConsensusTimeoutMs),
		rootchain.WithQuorumThreshold(config.QuorumThreshold))

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
