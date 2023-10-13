package cmd

import (
	"context"
	"crypto"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type (
	tokensConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
	}
)

func newTokensNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &tokensConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "tokens",
		Short: "Starts a User-Defined Token partition's node",
		Long:  `Starts a User-Defined Token partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokensNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "tokens")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runTokensNode(ctx context.Context, cfg *tokensConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return fmt.Errorf("loading partition genesis: %w", err)
	}

	trustBase, err := genesis.NewValidatorTrustBase(pg.RootValidators)
	if err != nil {
		return fmt.Errorf("creating trustbase: %w", err)
	}
	peer, err := createNetworkPeer(ctx, cfg.Node, pg, cfg.Base.Logger)
	if err != nil {
		return fmt.Errorf("creating network peer: %w", err)
	}
	log := cfg.Base.Logger.With(logger.NodeID(peer.ID()))

	txs, err := tokens.NewTxSystem(
		log,
		tokens.WithSystemIdentifier(pg.SystemDescriptionRecord.GetSystemIdentifier()),
		tokens.WithHashAlgorithm(crypto.SHA256),
		tokens.WithTrustBase(trustBase),
	)
	if err != nil {
		return fmt.Errorf("creating tx system: %w", err)
	}
	node, err := createNode(ctx, peer, txs, cfg.Node, nil, log)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "tokens node", peer, node, cfg.RPCServer, cfg.RESTServer, log)
}
