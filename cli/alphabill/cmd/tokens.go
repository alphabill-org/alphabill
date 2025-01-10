package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	tokenssdk "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	tokenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

type (
	tokensConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *rpc.ServerConfiguration
	}
)

func newTokensNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &tokensConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &rpc.ServerConfiguration{},
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

	return nodeCmd
}

func runTokensNode(ctx context.Context, cfg *tokensConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return fmt.Errorf("loading partition genesis: %w", err)
	}

	params := &genesis.TokensPartitionParams{}
	if err := types.Cbor.Unmarshal(pg.Params, params); err != nil {
		return fmt.Errorf("failed to unmarshal tokens partition params: %w", err)
	}

	stateFilePath := cfg.Node.StateFile
	if stateFilePath == "" {
		stateFilePath = filepath.Join(cfg.Base.HomeDir, utDir, utGenesisStateFileName)
	}
	state, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return tokenssdk.NewUnitData(ui, pg.PartitionDescription)
	})
	if err != nil {
		return fmt.Errorf("loading state (file %s): %w", cfg.Node.StateFile, err)
	}

	// Only genesis state can be uncommitted, try to commit
	if !state.IsCommitted() {
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

	// register all unit- and attribute types from token tx system
	enc, err := encoder.New(tokenc.RegisterTxAttributeEncoders, tokenc.RegisterUnitDataEncoders)
	if err != nil {
		return fmt.Errorf("creating encoders for WASM predicate engine: %w", err)
	}
	// create predicate engine for the transaction system.
	// however, the WASM engine needs to be able to execute predicate templates
	// so we create template engine first and use it for both WASM engine and
	// tx system engine.
	// it's safe to share template engine as it doesn't have internal state and
	// predicate executions happen in serialized manner anyway
	templateEng, err := templates.New(obs)
	if err != nil {
		return fmt.Errorf("creating predicate templates executor: %w", err)
	}
	tpe, err := predicates.Dispatcher(templateEng)
	if err != nil {
		return fmt.Errorf("creating predicate executor for WASM engine: %w", err)
	}
	wasmEng, err := wasm.New(enc, tpe.Execute, obs)
	if err != nil {
		return fmt.Errorf("creating predicate WASM executor: %w", err)
	}
	predEng, err := predicates.Dispatcher(templateEng, wasmEng)
	if err != nil {
		return fmt.Errorf("creating predicate executor: %w", err)
	}

	txs, err := tokens.NewTxSystem(
		*pg.PartitionDescription,
		types.ShardID{},
		obs,
		tokens.WithHashAlgorithm(crypto.SHA256),
		tokens.WithTrustBase(trustBase),
		tokens.WithState(state),
		tokens.WithPredicateExecutor(predEng.Execute),
		tokens.WithAdminOwnerPredicate(params.AdminOwnerPredicate),
		tokens.WithFeelessMode(params.FeelessMode),
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
	return run(ctx, node, cfg.RPCServer, ownerIndexer, obs)
}
