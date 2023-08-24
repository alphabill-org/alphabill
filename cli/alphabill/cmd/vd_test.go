package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRunVD(t *testing.T) {
	homeDirVD := setupTestHomeDir(t, "vd")
	keysFileLocation := filepath.Join(homeDirVD, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDirVD, nodeGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDirVD, "partition-genesis.json")
	testtime.MustRunInTime(t, 5*time.Second, func() {
		addr := "localhost:9543"
		conf := &vdConfiguration{
			baseNodeConfiguration: baseNodeConfiguration{
				Base: &baseConfiguration{
					HomeDir:    alphabillHomeDir(),
					CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
					LogCfgFile: defaultLoggerConfigFile,
				},
			},
			Node: &startNodeConfiguration{},
			RPCServer: &grpcServerConfiguration{
				Address:        defaultServerAddr,
				MaxRecvMsgSize: defaultMaxRecvMsgSize,
				MaxSendMsgSize: defaultMaxSendMsgSize,
			},
		}
		conf.RPCServer.Address = addr

		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New()
		args := "vd-genesis --home " + homeDirVD + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.addAndExecuteCommand(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{})
		require.NoError(t, err)

		// use same keys for signing and communication encryption.
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		rootPubKeyBytes, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
		require.NoError(t, err)
		_, partitionGenesisFiles, err := rootgenesis.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)

		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			cmd = New()
			args = "vd --home " + homeDirVD + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + addr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()

		log.Info("Started vd node and dialing...")
		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)

		// Test
		// green path
		id := types.NewUnitID(65, nil, test.RandomBytes(32), []byte{1})
		tx := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       vd.DefaultSystemIdentifier,
				Type:           vd.PayloadTypeRegisterData,
				UnitID:         id[:],
				ClientMetadata: &types.ClientMetadata{Timeout: 10},
			},
		}
		txBytes, _ := cbor.Marshal(tx)
		txProto := &alphabill.Transaction{Order: txBytes}

		_, err = rpcClient.ProcessTransaction(ctx, txProto, grpc.WaitForReady(true))
		require.NoError(t, err)

		// failing case
		tx.Payload.SystemID = []byte{0, 0, 0, 0} // incorrect system id
		txBytes, _ = cbor.Marshal(tx)
		txProto = &alphabill.Transaction{Order: txBytes}

		_, err = rpcClient.ProcessTransaction(ctx, txProto, grpc.WaitForReady(true))
		require.ErrorContains(t, err, "invalid transaction system identifier")

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
