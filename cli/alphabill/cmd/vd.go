package cmd

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"
	"github.com/spf13/cobra"
)

type (
	vdConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *grpcServerConfiguration
	}
)

func newVDNodeCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &vdConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &grpcServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "vd",
		Short: "Starts a Verifiable Data partition's node",
		Long:  `Starts a Verifiable Data partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVDNode(ctx, config)
		},
	}

	nodeCmd.Flags().StringVarP(&config.Node.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.Node.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringToStringVarP(&config.Node.Peers, "peers", "p", nil, "a map of partition peer identifiers and addresses. must contain all genesis validator addresses")
	nodeCmd.Flags().StringVarP(&config.Node.KeyFile, keyFileCmdFlag, "k", "", "path to the key file (default: $AB_HOME/vd/keys.json)")
	nodeCmd.Flags().StringVarP(&config.Node.Genesis, "genesis", "g", "", "path to the partition genesis file : $AB_HOME/vd/partition-genesis.json)")

	config.RPCServer.addConfigurationFlags(nodeCmd)

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
	return defaultNodeRunFunc(ctx, "vd node", txs, cfg.Node, cfg.RPCServer)
}
