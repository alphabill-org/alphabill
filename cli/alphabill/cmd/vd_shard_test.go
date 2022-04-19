package cmd

import (
	"context"
	"path"
	"sync"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const defaultUnicityTrustBase = "0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"

func TestVDShard(t *testing.T) {
	abApp := New()
	abApp.rootCmd.SetArgs([]string{"vd-shard", "--trust-base", defaultUnicityTrustBase})
	abApp.Execute(context.Background())
}

func TestRunVDShard(t *testing.T) {
	test.MustRunInTime(t, 5*time.Second, func() {
		port := "9543"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

		conf := &vdShardConfiguration{
			baseShardConfiguration: baseShardConfiguration{
				Root: &rootConfiguration{
					HomeDir:    alphabillHomeDir(),
					CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
					LogCfgFile: defaultLoggerConfigFile,
				},
				Server: &grpcServerConfiguration{
					Address:        defaultServerAddr,
					MaxRecvMsgSize: defaultMaxRecvMsgSize,
				},
			},
			UnicityTrustBase: []string{defaultUnicityTrustBase},
		}
		conf.Server.Address = listenAddr

		appStoppedWg := sync.WaitGroup{}
		ctx, _ := async.WithWaitGroup(context.Background())
		ctx, ctxCancel := context.WithCancel(ctx)

		// Starting the shard in background
		appStoppedWg.Add(1)
		go func() {
			err := defaultVDShardRunFunc(ctx, conf)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		log.Info("Started vd-shard and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, dialAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)

		// Test
		// green path
		id := uint256.NewInt(defaultInitialBillId).Bytes32()
		tx := &transaction.Transaction{
			UnitId:                id[:],
			TransactionAttributes: new(anypb.Any),
			Timeout:               1,
			OwnerProof:            script.PredicateArgumentEmpty(),
			SystemId:              []byte{1},
		}
		reg := &transaction.RegisterData{}
		err = anypb.MarshalFrom(tx.TransactionAttributes, reg, proto.MarshalOptions{})
		require.NoError(t, err)

		response, err := rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.NoError(t, err)
		require.True(t, response.Ok, "Successful response ok should be true")

		// failing case
		tx.SystemId = []byte{0} // incorrect system id
		reg = &transaction.RegisterData{}
		err = anypb.MarshalFrom(tx.TransactionAttributes, reg, proto.MarshalOptions{})
		require.NoError(t, err)

		response, err = rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.Error(t, err)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
