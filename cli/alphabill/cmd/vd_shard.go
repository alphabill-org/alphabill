package cmd

import (
	"context"
	"os"
	"path"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	vdtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"
	"github.com/spf13/cobra"
)

type (
	vdShardConfiguration struct {
		baseShardConfiguration
		// trust base public keys, in compressed secp256k1 (33 bytes each) hex format
		UnicityTrustBase []string
	}

	vdShardTxConverter struct{}
)

const vdFolder = "vd"

func newVDShardCmd(ctx context.Context, rootConfig *rootConfiguration) *cobra.Command {
	config := &vdShardConfiguration{
		baseShardConfiguration: baseShardConfiguration{
			Root:   rootConfig,
			Server: &grpcServerConfiguration{},
		},
	}

	var shardCmd = &cobra.Command{
		Use:   "vd-shard",
		Short: "Starts a Verifiable Data partition's shard node",
		Long:  `Starts a Verifiable Data partition's shard node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultVDShardRunFunc(ctx, config)
		},
	}

	shardCmd.Flags().StringSliceVar(&config.UnicityTrustBase, "trust-base", []string{}, "public key used as trust base, in compressed (33 bytes) hex format.")
	config.Server.addConfigurationFlags(shardCmd)

	return shardCmd
}

func (r *vdShardTxConverter) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return vdtx.NewVerifiableDataTx(tx)
}

func defaultVDShardRunFunc(ctx context.Context, cfg *vdShardConfiguration) error {
	state, err := verifiable_data.NewVerifiableDataTxSystem(cfg.UnicityTrustBase)
	if err != nil {
		return err
	}

	err = os.MkdirAll(path.Join(cfg.Root.HomeDir, vdFolder), 0700) // -rwx------
	if err != nil {
		return err
	}
	blockStoreFile := path.Join(cfg.Root.HomeDir, vdFolder, store.BoltBlockStoreFileName)
	blockStore, err := store.NewBoltBlockStore(blockStoreFile)
	if err != nil {
		return err
	}

	return defaultShardRunFunc(ctx, &cfg.baseShardConfiguration, &vdShardTxConverter{}, state, blockStore)
}
