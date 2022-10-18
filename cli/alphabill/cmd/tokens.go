package cmd

import (
	"context"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/spf13/cobra"
)

type (
	tokensConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *grpcServerConfiguration
	}
)

func newTokensNodeCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &tokensConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &grpcServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "tokens",
		Short: "Starts an User-Defined Token partition's node",
		Long:  `Starts an User-Defined Token partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokensNode(ctx, config)
		},
	}

	nodeCmd.Flags().StringVarP(&config.Node.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.Node.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringToStringVarP(&config.Node.Peers, "peers", "p", nil, "a map of partition peer identifiers and addresses. must contain all genesis validator addresses")
	nodeCmd.Flags().StringVarP(&config.Node.KeyFile, keyFileCmdFlag, "k", "", "path to the key file (default: $AB_HOME/tokens/keys.json)")
	nodeCmd.Flags().StringVarP(&config.Node.Genesis, "genesis", "g", "", "path to the partition genesis file : $AB_HOME/tokens/partition-genesis.json)")
	nodeCmd.Flags().StringVarP(&config.Node.DbFile, "db", "f", "", "path to the database file (default: $AB_HOME/tokens/"+store.BoltBlockStoreFileName+")")

	config.RPCServer.addConfigurationFlags(nodeCmd)

	return nodeCmd
}

func runTokensNode(ctx context.Context, cfg *tokensConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}

	trustBase, err := genesis.NewValidatorTrustBase(pg.RootValidators)
	if err != nil {
		return err
	}
	txs, err := tokens.New(
		tokens.WithSystemIdentifier(pg.SystemDescriptionRecord.GetSystemIdentifier()),
		tokens.WithHashAlgorithm(gocrypto.SHA256),
		tokens.WithTrustBase(trustBase),
	)
	if err != nil {
		return err
	}
	return defaultNodeRunFunc(ctx, "tokens node", txs, cfg.Node, cfg.RPCServer)
}
