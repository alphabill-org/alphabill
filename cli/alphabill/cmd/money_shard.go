package cmd

import (
	"context"
	"crypto"
	"os"
	"path"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"
	"github.com/holiman/uint256"
	"github.com/spf13/cobra"
)

type (
	moneyShardConfiguration struct {
		baseShardConfiguration
		// The value of initial bill in AlphaBills.
		InitialBillValue uint64 `validate:"gte=0"`
		// The initial value of Dust Collector Money supply.
		DCMoneySupplyValue uint64 `validate:"gte=0"`
		// trust base public keys, in compressed secp256k1 (33 bytes each) hex format
		UnicityTrustBase []string
	}

	moneyShardTxConverter struct{}

	// moneyShardRunnable is the function that is run after configuration is loaded.
	moneyShardRunnable func(ctx context.Context, shardConfig *moneyShardConfiguration) error
)

const (
	defaultInitialBillValue   = 1000000
	defaultDCMoneySupplyValue = 1000000
	defaultInitialBillId      = 1
)

var log = logger.CreateForPackage()

// newMoneyShardCmd creates a new cobra command for the shard component.
//
// shardRunFunc - set the function to override the default behaviour. Meant for tests.
func newMoneyShardCmd(ctx context.Context, rootConfig *rootConfiguration, shardRunFunc moneyShardRunnable) *cobra.Command {
	config := &moneyShardConfiguration{
		baseShardConfiguration: baseShardConfiguration{
			Root:   rootConfig,
			Server: &grpcServerConfiguration{},
		},
	}
	// shardCmd represents the shard command
	var shardCmd = &cobra.Command{
		Use:   "shard",
		Short: "Starts a shard node",
		Long:  `Shard a shard component, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shardRunFunc != nil {
				return shardRunFunc(ctx, config)
			}
			return defaultMoneyShardRunFunc(ctx, config)
		},
	}

	shardCmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value for new shard.")
	shardCmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	shardCmd.Flags().StringSliceVar(&config.UnicityTrustBase, "trust-base", []string{}, "public key used as trust base, in compressed (33 bytes) hex format.")
	config.Server.addConfigurationFlags(shardCmd)

	return shardCmd
}

func (r *moneyShardTxConverter) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return transaction.NewMoneyTx(tx)
}

func defaultMoneyShardRunFunc(ctx context.Context, config *moneyShardConfiguration) error {
	billsState, err := money.NewMoneySchemeState(crypto.SHA256, config.UnicityTrustBase, &money.InitialBill{
		ID:    uint256.NewInt(defaultInitialBillId),
		Value: config.InitialBillValue,
		Owner: script.PredicateAlwaysTrue(),
	}, config.DCMoneySupplyValue)
	if err != nil {
		return err
	}

	err = os.MkdirAll(config.Root.HomeDir, 0700) // -rwx------
	if err != nil {
		return err
	}
	blockStoreFile := path.Join(config.Root.HomeDir, store.BoltBlockStoreFileName)
	blockStore, err := store.NewBoltBlockStore(blockStoreFile)
	if err != nil {
		return err
	}

	return defaultShardRunFunc(ctx, &config.baseShardConfiguration, &moneyShardTxConverter{}, billsState, blockStore)
}
