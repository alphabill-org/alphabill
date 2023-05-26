package cmd

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/txsystem/program"
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

func newProgramsNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &programConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "programs",
		Short: "Starts a Programs partition's node",
		Long:  `Starts a Programs partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProgramNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "programs")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runProgramNode(ctx context.Context, cfg *programConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}
	txs, err := program.New(ctx, pg.SystemDescriptionRecord.GetSystemIdentifier())
	if err != nil {
		return err
	}
	return defaultNodeRunFunc(ctx, "programs node", txs, cfg.Node, cfg.RPCServer, cfg.RESTServer)
}
