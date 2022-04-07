package cmd

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"net"
)

type baseShardConfiguration struct {
	Root   *rootConfiguration
	Server *grpcServerConfiguration
}

func defaultShardRunFunc(ctx context.Context, cfg *baseShardConfiguration, converter shard.TxConverter, stateProcessor shard.StateProcessor) error {
	shardComponent, err := shard.New(converter, stateProcessor)
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
