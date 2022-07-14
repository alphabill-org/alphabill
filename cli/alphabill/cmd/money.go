package cmd

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/holiman/uint256"
	"github.com/spf13/cobra"
)

type (
	moneyNodeConfiguration struct {
		baseNodeConfiguration
		Node      *startNodeConfiguration
		RPCServer *grpcServerConfiguration
	}

	// moneyNodeRunnable is the function that is run after configuration is loaded.
	moneyNodeRunnable func(ctx context.Context, nodeConfig *moneyNodeConfiguration) error
)

var log = logger.CreateForPackage()

// newMoneyNodeCmd creates a new cobra command for the shard component.
//
// nodeRunFunc - set the function to override the default behaviour. Meant for tests.
func newMoneyNodeCmd(ctx context.Context, baseConfig *baseConfiguration, nodeRunFunc moneyNodeRunnable) *cobra.Command {
	config := &moneyNodeConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:      &startNodeConfiguration{},
		RPCServer: &grpcServerConfiguration{},
	}
	var nodeCmd = &cobra.Command{
		Use:   "money",
		Short: "Starts a money node",
		Long:  `Starts a money partition's node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if nodeRunFunc != nil {
				return nodeRunFunc(ctx, config)
			}
			return runMoneyNode(ctx, config)
		},
	}

	nodeCmd.Flags().StringVarP(&config.Node.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "node address in libp2p multiaddress-format")
	nodeCmd.Flags().StringVarP(&config.Node.RootChainAddress, "rootchain", "r", "/ip4/127.0.0.1/tcp/26662", "root chain address in libp2p multiaddress-format")
	nodeCmd.Flags().StringToStringVarP(&config.Node.Peers, "peers", "p", nil, "a map of partition peer identifiers and addresses. must contain all genesis validator addresses")
	nodeCmd.Flags().StringVarP(&config.Node.KeyFile, keyFileCmdFlag, "k", "", "path to the key file (default: $AB_HOME/money/keys.json)")
	nodeCmd.Flags().StringVarP(&config.Node.Genesis, "genesis", "g", "", "path to the partition genesis file : $AB_HOME/money/partition-genesis.json)")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runMoneyNode(ctx context.Context, cfg *moneyNodeConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return errors.Wrapf(err, "failed to read genesis file %s", cfg.Node.Genesis)
	}

	params := &genesis.MoneyPartitionParams{}
	err = pg.Params.UnmarshalTo(params)
	if err != nil {
		return err
	}

	ib := &money.InitialBill{
		ID:    uint256.NewInt(defaultInitialBillId),
		Value: params.InitialBillValue,
		Owner: script.PredicateAlwaysTrue(),
	}

	txs, err := money.NewMoneyTxSystem(
		crypto.SHA256,
		ib,
		params.DcMoneySupplyValue,
		money.SchemeOpts.SystemIdentifier(pg.GetSystemDescriptionRecord().GetSystemIdentifier()),
	)
	if err != nil {
		return errors.Wrapf(err, "failed to start money transaction system")
	}
	return defaultNodeRunFunc(ctx, "money node", txs, cfg.Node, cfg.RPCServer)
}
