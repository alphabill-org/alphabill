package cmd

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/txsystem/verifiable_data"
	"github.com/spf13/cobra"
)

type (
	vdConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
	}
)

func newVDNodeCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &vdConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "vd",
		Short: "Starts a Verifiable Data partition's node",
		Long:  `Starts a Verifiable Data partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVDNode(ctx, config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "vd")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runVDNode(ctx context.Context, cfg *vdConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}
	txs, err := verifiable_data.New(pg.SystemDescriptionRecord.GetSystemIdentifier())
	if err != nil {
		return err
	}
	return defaultNodeRunFunc(ctx, "vd node", txs, cfg.Node, cfg.RPCServer, cfg.RESTServer)
}
