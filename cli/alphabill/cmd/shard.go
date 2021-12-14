package cmd

import (
	"context"
	"crypto"
	"net"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"

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
		InitialBillValue uint32 `validate:"gte=0"`
	}
	// shardRunnable is the function that is run after configuration is loaded.
	shardRunnable func(ctx context.Context, shardConfig *shardConfiguration) error
)

const (
	defaultInitialBillValue = 1000000
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

	shardCmd.Flags().Uint32Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value for new shard")
	config.Server.addConfigurationFlags(shardCmd)

	return shardCmd
}

func defaultShardRunFunc(ctx context.Context, cfg *shardConfiguration) error {
	billsState, err := state.New(crypto.SHA256, []*state.BillContent{state.NewInitialBill(cfg.InitialBillValue, script.PredicateAlwaysTrue())})
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

	paymentServer, err := rpc.NewPaymentServer(shardComponent)
	if err != nil {
		return err
	}

	payment.RegisterPaymentsServer(grpcServer, paymentServer)

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
