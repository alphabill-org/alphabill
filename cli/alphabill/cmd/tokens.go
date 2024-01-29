package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

type (
	tokensConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		GRPCServer *grpcServerConfiguration
		RPCServer  *rpc.ServerConfiguration
	}
)

func newTokensNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &tokensConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		GRPCServer: &grpcServerConfiguration{},
		RPCServer:  &rpc.ServerConfiguration{},
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
	addRPCServerConfigurationFlags(nodeCmd, config.RPCServer)
	config.GRPCServer.addConfigurationFlags(nodeCmd)

	return nodeCmd
}

func runTokensNode(ctx context.Context, cfg *tokensConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return fmt.Errorf("loading partition genesis: %w", err)
	}

	stateFilePath := cfg.Node.StateFile
	if stateFilePath == "" {
		stateFilePath = filepath.Join(cfg.Base.HomeDir, utDir, utGenesisStateFileName)
	}
	state, err := loadStateFile(stateFilePath, tokens.NewUnitData)
	if err != nil {
		return fmt.Errorf("loading state (file %s): %w", cfg.Node.StateFile, err)
	}

	// Only genesis state can be uncommitted, try to commit
	if !state.IsCommitted() {
		if err := state.Commit(pg.Certificate); err != nil {
			return fmt.Errorf("invalid genesis state: %w", err)
		}
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

	log := cfg.Base.observe.Logger().With(logger.NodeID(nodeID))
	obs := observability.WithLogger(cfg.Base.observe, log)

	blockStore, err := initStore(cfg.Node.DbFile)
	if err != nil {
		return fmt.Errorf("unable to initialize block DB: %w", err)
	}

	proofStore, err := initStore(cfg.Node.TxIndexerDBFile)
	if err != nil {
		return fmt.Errorf("unable to initialize proof DB: %w", err)
	}

	txs, err := tokens.NewTxSystem(
		log,
		tokens.WithSystemIdentifier(pg.SystemDescriptionRecord.GetSystemIdentifier()),
		tokens.WithHashAlgorithm(crypto.SHA256),
		tokens.WithTrustBase(trustBase),
		tokens.WithState(state),
	)
	if err != nil {
		return fmt.Errorf("creating tx system: %w", err)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, obs)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "tokens node", node, cfg.GRPCServer, cfg.RPCServer, proofStore, obs)
}
