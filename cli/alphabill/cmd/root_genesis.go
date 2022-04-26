package cmd

import (
	"context"
	"encoding/binary"
	"fmt"
	abcrypto "gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"path"

	"github.com/spf13/cobra"
)

const partitionRecordFileCmd = "partition-record-file"

type (
	rootGenesisConfig struct {
		Root *rootConfiguration

		// paths to partition record json files
		PartitionRecordFiles []string

		// path to root chain key file
		KeyFile string

		// path to output directory where genesis files will be created (default current directory)
		OutputDir string
	}

	rootKey struct {
		PrivateKey string `json:"privateKey"`
	}
)

// newRootGenesisCmd creates a new cobra command for the root-genesis component.
func newRootGenesisCmd(ctx context.Context, rootConfig *rootConfiguration) *cobra.Command {
	config := &rootGenesisConfig{Root: rootConfig}
	var cmd = &cobra.Command{
		Use:   "root-genesis",
		Short: "Generates root chain genesis files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootGenesisRunFunc(ctx, config)
		},
	}

	cmd.Flags().StringVarP(&config.KeyFile, "key-file", "k", "", "path to root chain key file")
	cmd.Flags().StringSliceVarP(&config.PartitionRecordFiles, partitionRecordFileCmd, "p", []string{}, "path to partition record file")
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", alphabillHomeDir(), "path to output directory")

	err := cmd.MarkFlagRequired(partitionRecordFileCmd)
	if err != nil {
		panic(err)
	}
	return cmd
}

func rootGenesisRunFunc(_ context.Context, config *rootGenesisConfig) error {
	pr, err := loadPartitionRecords(config.PartitionRecordFiles)
	if err != nil {
		return err
	}
	k, err := loadKey(config.KeyFile)
	if err != nil {
		return err
	}
	rg, pg, err := rootchain.NewGenesis(pr, k)
	if err != nil {
		return err
	}
	err = saveRootGenesisFile(rg, config.OutputDir)
	if err != nil {
		return err
	}
	err = savePartitionGenesisFiles(pg, config.OutputDir)
	if err != nil {
		return err
	}
	return nil
}

func loadPartitionRecords(paths []string) ([]*genesis.PartitionRecord, error) {
	var prs []*genesis.PartitionRecord
	for _, p := range paths {
		pr, err := util.ReadJsonFile(p, &genesis.PartitionRecord{})
		if err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

func loadKey(path string) (abcrypto.Signer, error) {
	if path == "" {
		return abcrypto.NewInMemorySecp256K1Signer()
	}

	rk, err := util.ReadJsonFile(path, &rootKey{})
	if err != nil {
		return nil, err
	}
	rkBytes, err := hexutil.Decode(rk.PrivateKey)
	if err != nil {
		return nil, err
	}
	return abcrypto.NewInMemorySecp256K1SignerFromKey(rkBytes)
}

func saveRootGenesisFile(rg *genesis.RootGenesis, outputDir string) error {
	outputFile := path.Join(outputDir, "root-genesis.json")
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
