package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	evmsdk "github.com/alphabill-org/alphabill-go-sdk/txsystem/evm"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

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
	Base             *baseConfiguration
	Keys             *keysConfig
	SystemIdentifier types.SystemID
	Output           string
	OutputState      string
	T2Timeout        uint32
	BlockGasLimit    uint64
	GasUnitPrice     uint64
}

// newEvmGenesisCmd creates a new cobra command for the evm genesis.
func newEvmGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	var systemID uint32
	config := &evmGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, evmDir)}
	var cmd = &cobra.Command{
		Use:   "evm-genesis",
		Short: "Generates a genesis file for the evm partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SystemIdentifier = types.SystemID(systemID)
			return evmGenesisRunFun(cmd.Context(), config)
		},
	}

	addSystemIDFlag(cmd, &systemID, evmsdk.DefaultSystemID)
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/evm/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/evm/node-genesis-state.cbor)")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().Uint64Var(&config.BlockGasLimit, "gas-limit", evm.DefaultBlockGasLimit, "max units of gas processed in each block")
	cmd.Flags().Uint64Var(&config.GasUnitPrice, "gas-price", evm.DefaultGasPrice, "gas unit price in wei")
	config.Keys.addCmdFlags(cmd)
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

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("load keys %v failed: %w", config.Keys.GetKeyFileLocation(), err)
	}

	genesisState := state.NewEmptyState()

	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}
	params, err := config.getPartitionParams()
	if err != nil {
		return fmt.Errorf("failes to set evm genesis parameters: %w", err)
	}

	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
		partition.WithT2Timeout(config.T2Timeout),
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
