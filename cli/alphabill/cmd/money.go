package cmd

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/alphabill-org/alphabill/pkg/observability"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

type (
	moneyNodeConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
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
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
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

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
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

	ib := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: params.InitialBillValue,
		Owner: templates.AlwaysTrueBytes(),
	}

	state, err := loadStateFile(cfg.Node.StateFile, money.NewUnitData)
	if err != nil {
		return fmt.Errorf("loading state (file %s): %w", cfg.Node.StateFile, err)
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
		log,
		money.WithSystemIdentifier(pg.SystemDescriptionRecord.SystemIdentifier),
		money.WithHashAlgorithm(crypto.SHA256),
		money.WithInitialBill(ib),
		money.WithState(state),
		money.WithSystemDescriptionRecords(params.SystemDescriptionRecords),
		money.WithDCMoneyAmount(params.DcMoneySupplyValue),
		money.WithTrustBase(trustBase),
	)
	if err != nil {
		return fmt.Errorf("creating money transaction system: %w", err)
	}
	node, err := createNode(ctx, txs, cfg.Node, keys, blockStore, proofStore, obs)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	return run(ctx, "money node", node, cfg.RPCServer, cfg.RESTServer, proofStore, obs)
}
