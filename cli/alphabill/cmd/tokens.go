package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/money"
	moneyenc "github.com/alphabill-org/alphabill/txsystem/money/encoder"
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

	enc, err := encoder.New(
		// register all unit- and attribute types from token tx system
		tokenc.RegisterTxAttributeEncoders, tokenc.RegisterUnitDataEncoders,
		// from the money tx system we only need/want to support transfer transactions (ie to check
		// has money transfer taken place, other money tx types shouldn't be needed by token predicates)
		moneyenc.RegisterTxAttributeEncodersF(func(id encoder.AttrEncID) bool {
			return slices.Contains([]string{money.PayloadTypeTransfer, money.PayloadTypeSplit}, id.Attr)
		}),
	)
	if err != nil {
		return fmt.Errorf("creating encoders for WASM predicate engine: %w", err)
	}
	predEng, err := predicates.Dispatcher(templates.New(), wasm.New(enc, obs))
	if err != nil {
		return fmt.Errorf("creating predicate executor: %w", err)
	}

	txs, err := tokens.NewTxSystem(
		obs,
		tokens.WithSystemIdentifier(pg.SystemDescriptionRecord.GetSystemIdentifier()),
		tokens.WithHashAlgorithm(crypto.SHA256),
		tokens.WithTrustBase(trustBase),
		tokens.WithState(state),
		tokens.WithPredicateExecutor(predEng.Execute),
	)
	if err != nil {
		return fmt.Errorf("creating tx system: %w", err)
	}
	var ownerIndexer *partition.OwnerIndexer
	if cfg.Node.WithOwnerIndex {
		ownerIndexer = partition.NewOwnerIndexer(log)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, ownerIndexer, obs)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "tokens node", node, cfg.RPCServer, ownerIndexer, obs)
}
