package cmd

import (
	"context"
	"crypto"
	"net"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"github.com/holiman/uint256"
	"github.com/spf13/cobra"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type (
	shardConfiguration struct {
		Root   *rootConfiguration
		Server *grpcServerConfiguration
		// The value of initial bill in AlphaBills.
		InitialBillValue uint64 `validate:"gte=0"`
		// The initial value of Dust Collector Money supply.
		DCMoneySupplyValue uint64 `validate:"gte=0"`
		// trust base public keys, in compressed secp256k1 (33 bytes each) hex format
		UnicityTrustBase []string
	}
	// shardRunnable is the function that is run after configuration is loaded.
	shardRunnable func(ctx context.Context, shardConfig *shardConfiguration) error
)

const (
	defaultInitialBillValue   = 1000000
	defaultDCMoneySupplyValue = 1000000
	defaultInitialBillId      = 1
)

var log = logger.CreateForPackage()

// newShardCmd creates a new cobra command for the shard component.
//
// shardRunFunc - set the function to override the default behaviour. Meant for tests.
func newShardCmd(ctx context.Context, rootConfig *rootConfiguration, shardRunFunc shardRunnable) *cobra.Command {
	config := &shardConfiguration{
		Root:   rootConfig,
		Server: &grpcServerConfiguration{},
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
			return defaultShardRunFunc(ctx, config)
		},
	}

	shardCmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value for new shard.")
	shardCmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	shardCmd.Flags().StringSliceVar(&config.UnicityTrustBase, "trust-base", []string{}, "public key used as trust base, in compressed (33 bytes) hex format.")
	config.Server.addConfigurationFlags(shardCmd)

	return shardCmd
}

func defaultShardRunFunc(ctx context.Context, cfg *shardConfiguration) error {
	billsState, err := txsystem.NewMoneySchemeState(crypto.SHA256, cfg.UnicityTrustBase, &txsystem.InitialBill{
		ID:    uint256.NewInt(defaultInitialBillId),
		Value: cfg.InitialBillValue,
		Owner: script.PredicateAlwaysTrue(),
	}, cfg.DCMoneySupplyValue)
	if err != nil {
		return err
	}

	shardComponent, err := shard.New(billsState)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.Server.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.Server.GrpcKeepAliveServerParameters()),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	listener, err := net.Listen("tcp", cfg.Server.Address)
	if err != nil {
		return err
	}

	transactionsServer, err := rpc.NewTransactionsServer(shardComponent)
	if err != nil {
		return err
	}

	transaction.RegisterTransactionsServer(grpcServer, transactionsServer)

	starterFunc := func(ctx context.Context) {
		go func() {
			log.Info("Starting gRPC server on %s", cfg.Server.Address)
			err = grpcServer.Serve(listener)
			if err != nil {
				log.Error("Server exited with erroneous situation: %s", err)
				return
			}
			log.Info("Server exited successfully")
		}()
		<-ctx.Done()
		grpcServer.GracefulStop()
	}

	return starter.StartAndWait(ctx, "shard", starterFunc)
}
