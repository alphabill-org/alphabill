package cmd

import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/api"
)

type (
	evmConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *rpc.ServerConfiguration
	}
)

func newEvmNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &evmConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &rpc.ServerConfiguration{},
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
	addRPCServerConfigurationFlags(nodeCmd, config.RPCServer)

	// mark the --tb-tx flag as mandatory for EVM nodes
	if err := nodeCmd.MarkFlagRequired("tx-db"); err != nil {
		panic(err)
	}
	return nodeCmd
}

func runEvmNode(ctx context.Context, cfg *evmConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}
	params := &genesis.EvmPartitionParams{}
	if err = types.Cbor.Unmarshal(pg.Params, params); err != nil {
		return fmt.Errorf("failed to unmarshal evm partition params: %w", err)
	}

	stateFilePath := cfg.Node.StateFile
	if stateFilePath == "" {
		stateFilePath = filepath.Join(cfg.Base.HomeDir, evmDir, evmGenesisStateFileName)
	}
	state, err := loadStateFile(stateFilePath, evm.NewUnitData)
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

	blockStore, err := initStore(cfg.Node.DbFile)
	if err != nil {
		return fmt.Errorf("unable to initialize block DB: %w", err)
	}

	proofStore, err := initStore(cfg.Node.TxIndexerDBFile)
	if err != nil {
		return fmt.Errorf("unable to initialize proof DB: %w", err)
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

	partitionID := pg.PartitionDescription.GetPartitionID()
	txs, err := evm.NewEVMTxSystem(
		pg.PartitionDescription.GetNetworkID(),
		partitionID,
		obs,
		evm.WithBlockGasLimit(params.BlockGasLimit),
		evm.WithGasPrice(params.GasUnitPrice),
		evm.WithBlockDB(blockStore),
		evm.WithTrustBase(trustBase),
		evm.WithState(state),
	)
	if err != nil {
		return fmt.Errorf("evm transaction system init failed: %w", err)
	}
	var ownerIndexer *partition.OwnerIndexer
	if cfg.Node.WithOwnerIndex {
		ownerIndexer = partition.NewOwnerIndexer(log)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, ownerIndexer, trustBase, obs)
	if err != nil {
		return fmt.Errorf("failed to create node evm node: %w", err)
	}
	cfg.RPCServer.Router = api.NewAPI(
		state,
		partitionID,
		big.NewInt(0).SetUint64(params.BlockGasLimit),
		params.GasUnitPrice,
		log,
	)
	return run(ctx, node, cfg.RPCServer, ownerIndexer, obs)
}
