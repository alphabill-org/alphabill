package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

type (
	moneyNodeConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		rpcServer *rpc.ServerConfiguration
	}

	// moneyNodeRunnable is the function that is run after configuration is loaded.
	moneyNodeRunnable func(ctx context.Context, nodeConfig *moneyNodeConfiguration) error
)

// newMoneyNodeCmd creates a new cobra command for the shard component.
//
// nodeRunFunc - set the function to override the default behavior. Meant for tests.
func newMoneyNodeCmd(baseConfig *baseConfiguration, nodeRunFunc moneyNodeRunnable) *cobra.Command {
	config := &moneyNodeConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		rpcServer: &rpc.ServerConfiguration{},
	}
	var nodeCmd = &cobra.Command{
		Use:   "money",
		Short: "Starts a money node",
		Long:  `Starts a money partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if nodeRunFunc != nil {
				return nodeRunFunc(cmd.Context(), config)
			}
			return runMoneyNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "money")
	addRPCServerConfigurationFlags(nodeCmd, config.rpcServer)
	return nodeCmd
}

func runMoneyNode(ctx context.Context, cfg *moneyNodeConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return fmt.Errorf("loading partition genesis (file %s): %w", cfg.Node.Genesis, err)
	}

	params := &genesis.MoneyPartitionParams{}
	if err := cbor.Unmarshal(pg.Params, params); err != nil {
		return fmt.Errorf("failed to unmarshal money partition params: %w", err)
	}

	stateFilePath := cfg.Node.StateFile
	if stateFilePath == "" {
		stateFilePath = filepath.Join(cfg.Base.HomeDir, moneyPartitionDir, moneyGenesisStateFileName)
	}
	state, err := loadStateFile(stateFilePath, money.NewUnitData)
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
		return fmt.Errorf("failed to create trust base validator: %w", err)
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

	txs, err := money.NewTxSystem(
		obs,
		money.WithSystemIdentifier(pg.SystemDescriptionRecord.SystemIdentifier),
		money.WithHashAlgorithm(crypto.SHA256),
		money.WithSystemDescriptionRecords(params.SystemDescriptionRecords),
		money.WithTrustBase(trustBase),
		money.WithState(state),
	)
	if err != nil {
		return fmt.Errorf("creating money transaction system: %w", err)
	}
	var ownerIndexer *partition.OwnerIndexer
	if cfg.Node.WithOwnerIndex {
		ownerIndexer = partition.NewOwnerIndexer(log)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, ownerIndexer, obs)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "money node", node, cfg.rpcServer, ownerIndexer, obs)
}
