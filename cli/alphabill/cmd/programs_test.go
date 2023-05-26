package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestRunPrograms(t *testing.T) {
	homeDir := setupHome(t, programsDir)
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, nodeGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "partition-genesis.json")
	testtime.MustRunInTime(t, 5*time.Second, func() {
		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New()
		args := "programs-genesis --home " + programsDir + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
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

		listenAddr := fmt.Sprintf(":%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg.Add(1)
		go func() {

			cmd = New()
			args = "programs --home " + programsDir + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()
		// Create the gRPC client
		log.Info("Started programs node")
		conn, err := grpc.DialContext(ctx, "localhost"+listenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)
		tx := &txsystem.Transaction{
			UnitId:                make([]byte, 32),
			TransactionAttributes: new(anypb.Any),
			ClientMetadata:        &txsystem.ClientMetadata{Timeout: 10},
			OwnerProof:            nil,
			SystemId:              program.DefaultProgramsSystemIdentifier,
		}
		call := &program.PCallAttributes{
			Function: "test",
			Input:    []byte{},
		}
		err = anypb.MarshalFrom(tx.TransactionAttributes, call, proto.MarshalOptions{})
		require.NoError(t, err)
		_, err = rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		// tx is sent, but it will fail as the program does not exist, this is just to make sure that node has started
		require.NoError(t, err)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
