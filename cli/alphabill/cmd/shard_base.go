package cmd

import (
	"context"
	"net"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type baseShardConfiguration struct {
	Root   *rootConfiguration
	Server *grpcServerConfiguration
}

func defaultShardRunFunc(ctx context.Context, cfg *baseShardConfiguration, converter shard.TxConverter, stateProcessor shard.StateProcessor, blockStore store.BlockStore) error {
	shardComponent, err := shard.New(converter, stateProcessor, blockStore)
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

	rpcServer, err := rpc.NewRpcServer(shardComponent, shardComponent)
	if err != nil {
		return err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)

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
