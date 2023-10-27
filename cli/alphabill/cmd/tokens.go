package cmd

import (
	"context"
	"crypto"
	"fmt"

	tokens2 "github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/common/logger"
	"github.com/alphabill-org/alphabill/validator/pkg/network/protocol/genesis"
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

	keys, err := LoadKeys(cfg.Node.KeyFile, false, false)
	if err != nil {
		return fmt.Errorf("failed to load node keys: %w", err)
	}

	nodeID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to calculate nodeID: %w", err)
	}

	log := cfg.Base.Logger.With(logger.NodeID(nodeID))

	txs, err := tokens2.NewTxSystem(
		log,
		tokens2.WithSystemIdentifier(pg.SystemDescriptionRecord.GetSystemIdentifier()),
		tokens2.WithHashAlgorithm(crypto.SHA256),
		tokens2.WithTrustBase(trustBase),
	)
	if err != nil {
		return fmt.Errorf("creating tx system: %w", err)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, nil, log)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "tokens node", node, cfg.RPCServer, cfg.RESTServer, log)
}
