package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	sdkorchestration "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"
)

type (
	orchestrationConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *rpc.ServerConfiguration
	}
)

func newOrchestrationNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &orchestrationConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &rpc.ServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "orchestration",
		Short: "Starts a Orchestration partition node",
		Long:  `Starts a Orchestration partition node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrchestrationNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "orchestration")
	addRPCServerConfigurationFlags(nodeCmd, config.RPCServer)

	return nodeCmd
}

func runOrchestrationNode(ctx context.Context, cfg *orchestrationConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return fmt.Errorf("loading partition genesis: %w", err)
	}

	var params *genesis.OrchestrationPartitionParams
	if err := types.Cbor.Unmarshal(pg.Params, &params); err != nil {
		return fmt.Errorf("failed to unmarshal orchestration partition params: %w", err)
	}

	stateFilePath := cfg.Node.StateFile
	if stateFilePath == "" {
		stateFilePath = filepath.Join(cfg.Base.HomeDir, orchestrationPartitionDir, orchestrationGenesisFileName)
	}
	state, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return sdkorchestration.NewUnitData(ui, pg.PartitionDescription)
	})
	if err != nil {
		return fmt.Errorf("loading state (file %s): %w", cfg.Node.StateFile, err)
	}

	// Only genesis state can be uncommitted, try to commit
	committed, err := state.IsCommitted()
	if err != nil {
		return fmt.Errorf("failed to check if state is committed: %w", err)
	}
	if !committed {
		if err := state.Commit(pg.Certificate); err != nil {
			return fmt.Errorf("invalid genesis state: %w", err)
		}
	}

	trustBase, err := types.NewTrustBaseFromFile(cfg.Node.TrustBaseFile)
	if err != nil {
		return fmt.Errorf("failed to load trust base file: %w", err)
	}

	keys, err := LoadKeys(cfg.Node.KeyFile, false, false)
	if err != nil {
		return fmt.Errorf("failed to load node keys: %w", err)
	}

	nodeID, err := peer.IDFromPublicKey(keys.AuthPrivKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to calculate nodeID: %w", err)
	}

	log := cfg.Base.observe.Logger().With(logger.NodeID(nodeID), logger.Shard(pg.PartitionDescription.PartitionID, types.ShardID{}))
	obs := observability.WithLogger(cfg.Base.observe, log)

	blockStore, err := initStore(cfg.Node.DbFile)
	if err != nil {
		return fmt.Errorf("unable to initialize block DB: %w", err)
	}

	proofStore, err := initStore(cfg.Node.TxIndexerDBFile)
	if err != nil {
		return fmt.Errorf("unable to initialize proof DB: %w", err)
	}

	txs, err := orchestration.NewTxSystem(
		*pg.PartitionDescription,
		types.ShardID{},
		obs,
		orchestration.WithHashAlgorithm(crypto.SHA256),
		orchestration.WithTrustBase(trustBase),
		orchestration.WithState(state),
		orchestration.WithOwnerPredicate(params.OwnerPredicate),
	)
	if err != nil {
		return fmt.Errorf("creating transaction system: %w", err)
	}
	var ownerIndexer *partition.OwnerIndexer
	if cfg.Node.WithOwnerIndex {
		ownerIndexer = partition.NewOwnerIndexer(log)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, ownerIndexer, trustBase, obs)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, node, cfg.RPCServer, ownerIndexer, cfg.Node.WithGetUnits, pg.PartitionDescription, obs, cfg.Node.StateRpcRateLimit)
}
