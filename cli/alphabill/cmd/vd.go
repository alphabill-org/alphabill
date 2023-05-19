package cmd

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/spf13/cobra"
)

const defaultVDNodeURL = "localhost:27766"

type (
	vdConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
	}
)

func newVDNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
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
			return runVDNode(cmd.Context(), config)
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
	txs, err := vd.NewTxSystem(pg.SystemDescriptionRecord.GetSystemIdentifier())
	if err != nil {
		return err
	}
	return defaultNodeRunFunc(ctx, "vd node", txs, cfg.Node, cfg.RPCServer, cfg.RESTServer)
}
