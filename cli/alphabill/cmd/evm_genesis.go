package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/common/util"
	evm2 "github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/partition"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	evmGenesisFileName = "node-genesis.json"
	evmDir             = "evm"
)

type evmGenesisConfig struct {
	Base             *baseConfiguration
	Keys             *keysConfig
	SystemIdentifier []byte
	Output           string
	T2Timeout        uint32
	BlockGasLimit    uint64
	GasUnitPrice     uint64
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

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", evm2.DefaultEvmTxSystemIdentifier, "system identifier in HEX format")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/evm/node-genesis.json)")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().Uint64Var(&config.BlockGasLimit, "gas-limit", evm2.DefaultBlockGasLimit, "max units of gas processed in each block")
	cmd.Flags().Uint64Var(&config.GasUnitPrice, "gas-price", evm2.DefaultGasPrice, "gas unit price in wei")
	config.Keys.addCmdFlags(cmd)
	return cmd
}

func (c *evmGenesisConfig) getPartitionParams() ([]byte, error) {
	if c.GasUnitPrice > math.MaxInt64 {
		return nil, fmt.Errorf("gas unit price too big")
	}

	src := &genesis.EvmPartitionParams{
		BlockGasLimit: c.BlockGasLimit,
		GasUnitPrice:  c.GasUnitPrice,
	}
	res, err := cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal money partition params: %w", err)
	}
	return res, nil
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
		return fmt.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("load keys %v failed: %w", config.Keys.GetKeyFileLocation(), err)
	}

	txSystem, err := evm2.NewEVMTxSystem(config.SystemIdentifier, config.Base.Logger)
	if err != nil {
		return err
	}

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
		txSystem,
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
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *evmGenesisConfig) getNodeGenesisFileLocation(homePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(homePath, evmGenesisFileName)
}
