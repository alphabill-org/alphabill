package cmd

import (
	"context"
	"math/rand"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	testtime "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRunVD(t *testing.T) {
	homeDirVD := setupTestHomeDir(t, "vd")
	keysFileLocation := path.Join(homeDirVD, keysFile)
	nodeGenesisFileLocation := path.Join(homeDirVD, nodeGenesisFileName)
	partitionGenesisFileLocation := path.Join(homeDirVD, "partition-genesis.json")
	testtime.MustRunInTime(t, 5*time.Second, func() {
		port := "9543"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

		conf := &vdConfiguration{
			baseNodeConfiguration: baseNodeConfiguration{
				Base: &baseConfiguration{
					HomeDir:    alphabillHomeDir(),
					CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
					LogCfgFile: defaultLoggerConfigFile,
				},
			},
			Node: &startNodeConfiguration{},
			RPCServer: &grpcServerConfiguration{
				Address:        defaultServerAddr,
				MaxRecvMsgSize: defaultMaxRecvMsgSize,
			},
		}
		conf.RPCServer.Address = listenAddr

		appStoppedWg := sync.WaitGroup{}
		ctx, _ := async.WithWaitGroup(context.Background())
		ctx, ctxCancel := context.WithCancel(ctx)

		// generate node genesis
		cmd := New()
		args := "vd-genesis --home " + homeDirVD + " -o " + nodeGenesisFileLocation + " -f -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.addAndExecuteCommand(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{})
		require.NoError(t, err)

		// use same keys for signing and communication encryption.
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		_, partitionGenesisFiles, err := rootchain.NewGenesisFromPartitionNodes([]*genesis.PartitionNode{pn}, 2500, rootSigner, verifier)
		require.NoError(t, err)

		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		// start the node in background
		appStoppedWg.Add(1)
		go func() {

			cmd = New()
			args = "vd-node --home " + homeDirVD + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.addAndExecuteCommand(ctx)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		log.Info("Started vd-node and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, dialAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)

		// Test
		// green path
		id := uint256.NewInt(rand.Uint64()).Bytes32()
		tx := &txsystem.Transaction{
			UnitId:                id[:],
			TransactionAttributes: nil,
			Timeout:               10,
			SystemId:              []byte{0, 0, 0, 1},
		}

		response, err := rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.NoError(t, err)
		require.True(t, response.Ok, "Successful response ok should be true")

		// failing case
		tx.SystemId = []byte{0, 0, 0, 0} // incorrect system id
		response, err = rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.Error(t, err)

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
