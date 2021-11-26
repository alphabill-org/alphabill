package main

import (
	"context"
	"crypto"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/bsn"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
)

type starter struct {
	start func(context.Context) error
}

func (s starter) Start(ctx context.Context) {
	if err := s.start(ctx); err != nil {
		err = errors.Wrap(err, "failed to start Bill Shard Node")
		log.Error("%s", err)
		panic(err)
	}
}

func runBillShardNode(ctx context.Context, config *configuration) error {
	bsnCli, err := cli.New("bsn", config, func(ctx context.Context) (cli.ComponentStarter, error) {

		billsState, err := state.New(crypto.SHA256, []*state.BillContent{state.NewInitialBill(config.InitialBillValue, script.PredicateAlwaysTrue())})
		if err != nil {
			return nil, err
		}

		bsnComponent, err := bsn.New(billsState)
		if err != nil {
			return nil, err
		}

		grpcServer := grpc.NewServer(
			grpc.MaxSendMsgSize(config.Server.MaxRecvMsgSize),
			grpc.KeepaliveParams(config.Server.GrpcKeepAliveServerParameters()),
		)
		grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

		listener, err := net.Listen("tcp", config.Server.Address)
		if err != nil {
			return nil, err
		}

		paymentServer, err := rpc.New(bsnComponent)
		if err != nil {
			return nil, err
		}

		payment.RegisterPaymentsServer(grpcServer, paymentServer)

		return &starter{func(ctx context.Context) error {
			go func() {
				err = grpcServer.Serve(listener)
				if err != nil {
					log.Error("Server exited with erroneous situation: %s", err)
					return
				}
				log.Info("Server exited successfully")
			}()
			<-ctx.Done()
			grpcServer.GracefulStop()
			return nil
		}}, nil
	})
	if err != nil {
		return err
	}

	return bsnCli.StartAndWait(ctx)
}
