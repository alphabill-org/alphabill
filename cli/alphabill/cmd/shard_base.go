package cmd

import (
	"context"
	"net"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/starter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type baseNodeConfiguration struct {
	Base   *baseConfiguration
	Server *grpcServerConfiguration
}

func defaultShardRunFunc(ctx context.Context, cfg *baseNodeConfiguration, converter shard.TxConverter, stateProcessor shard.StateProcessor, blockStore store.BlockStore) error {
	nodeComponent, err := shard.New(converter, stateProcessor, blockStore)
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

	rpcServer, err := rpc.NewRpcServer(nodeComponent, nodeComponent)
	if err != nil {
		return err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)

	wg, err := async.WaitGroup(ctx)
	if err != nil {
		return err
	}
	starterFunc := func(ctx context.Context) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("Starting gRPC server on %s", cfg.Server.Address)
			err = grpcServer.Serve(listener)
			if err != nil {
				log.Error("Server exited with erroneous situation: %s", err)
			} else {
				log.Info("Server exited successfully")
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			// ctx is cancelled when receiving SIGTERM in StartAndWait
			// this goroutine has to be started inside starterFunc as cancel context is added inside starterFunc
			<-ctx.Done()
			log.Info("Stopping gRPC server on %s", cfg.Server.Address)
			grpcServer.GracefulStop()
		}()
	}

	// startAndWaits waits until ctx.waitgroup is done OR sigterm cancels signal OR timeout (not used here)
	return starter.StartAndWait(ctx, "shard", starterFunc)
}
