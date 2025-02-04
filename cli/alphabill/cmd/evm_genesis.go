package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/evm"
)

const (
	evmGenesisFileName      = "node-genesis.json"
	evmGenesisStateFileName = "node-genesis-state.cbor"
	evmDir                  = "evm"
)

type evmGenesisConfig struct {
	Base          *baseConfiguration
	Keys          *keysConfig
	PDRFilename   string
	Output        string
	OutputState   string
	BlockGasLimit uint64
	GasUnitPrice  uint64
}

// newEvmGenesisCmd creates a new cobra command for the evm genesis.
func newEvmGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &evmGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, evmDir)}
	var cmd = &cobra.Command{
		Use:   "evm-genesis",
		Short: "Generates a genesis file for the evm partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return evmGenesisRunFun(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVar(&config.PDRFilename, "partition-description", "", "filename (full path) from where to read the Partition Description Record")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/evm/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/evm/node-genesis-state.cbor)")
	cmd.Flags().Uint64Var(&config.BlockGasLimit, "gas-limit", evm.DefaultBlockGasLimit, "max units of gas processed in each block")
	cmd.Flags().Uint64Var(&config.GasUnitPrice, "gas-price", evm.DefaultGasPrice, "gas unit price in wei")
	config.Keys.addCmdFlags(cmd)
	_ = cmd.MarkFlagRequired("partition-description")
	return cmd
}

func evmGenesisRunFun(_ context.Context, config *evmGenesisConfig) error {
	homePath := filepath.Join(config.Base.HomeDir, evmDir)
	if !util.FileExists(homePath) {
		err := os.MkdirAll(homePath, 0700) // -rwx------)
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(homePath)
	if util.FileExists(nodeGenesisFile) {
		return fmt.Errorf("node genesis file %q already exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	nodeGenesisStateFile := config.getNodeGenesisStateFileLocation(homePath)
	if util.FileExists(nodeGenesisStateFile) {
		return fmt.Errorf("node genesis state file %q already exists", nodeGenesisStateFile)
	}

	pdr, err := util.ReadJsonFile(config.PDRFilename, &types.PartitionDescriptionRecord{Version: 1})
	if err != nil {
		return fmt.Errorf("loading partition description: %w", err)
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("load keys %v failed: %w", config.Keys.GetKeyFileLocation(), err)
	}

	genesisState := state.NewEmptyState()

	peerID, err := peer.IDFromPublicKey(keys.AuthPrivKey.GetPublic())
	if err != nil {
		return err
	}
	params, err := config.getPartitionParams()
	if err != nil {
		return fmt.Errorf("failed to set evm genesis parameters: %w", err)
	}

	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		*pdr,
		partition.WithPeerID(peerID),
		partition.WithSigner(keys.Signer),
		partition.WithParams(params),
	)
	if err != nil {
		return err
	}

	if err := writeStateFile(nodeGenesisStateFile, genesisState); err != nil {
		return fmt.Errorf("failed to write genesis state file: %w", err)
	}

	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *evmGenesisConfig) getPartitionParams() ([]byte, error) {
	if c.GasUnitPrice > math.MaxInt64 {
		return nil, fmt.Errorf("gas unit price too big")
	}

	src := &genesis.EvmPartitionParams{
		BlockGasLimit: c.BlockGasLimit,
		GasUnitPrice:  c.GasUnitPrice,
	}
	res, err := types.Cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal money partition params: %w", err)
	}
	return res, nil
}

func (c *evmGenesisConfig) getNodeGenesisFileLocation(homePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(homePath, evmGenesisFileName)
}

func (c *evmGenesisConfig) getNodeGenesisStateFileLocation(homePath string) string {
	if c.OutputState != "" {
		return c.OutputState
	}
	return filepath.Join(homePath, evmGenesisStateFileName)
}
