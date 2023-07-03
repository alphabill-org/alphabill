package cmd

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/cobra"
)

type (
	programConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
	}
)

func newEvmNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &programConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "evm",
		Short: "Starts an evm partition node",
		Long:  `Starts an evm partition node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvmNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "evm")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runEvmNode(ctx context.Context, cfg *programConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}
	params := &genesis.EvmPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	if err != nil {
		return fmt.Errorf("failed to unmarshal evm partition params: %w", err)
	}
	txs, err := evm.NewEVMTxSystem(
		pg.SystemDescriptionRecord.GetSystemIdentifier(),
		evm.WithBlockGasLimit(params.BlockGasLimit),
		evm.WithGasPrice(params.GasUnitPrice),
	)
	if err != nil {
		return err
	}
	return defaultNodeRunFunc(ctx, "evm node", txs, cfg.Node, cfg.RPCServer, cfg.RESTServer)
}
