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
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
		makeProgramDeploy(t, ctx, rpcClient)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func makeProgramDeploy(t *testing.T, ctx context.Context, txClient alphabill.AlphabillServiceClient) {
	// this is a not valid application, but the tx is accepted as it is and proves that the node is running
	attr := &program.PDeployAttributes{
		ProgModule: []byte{1, 2, 3},
		ProgParams: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	}
	attrBytes, _ := cbor.Marshal(attr)
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           program.ProgramDeploy,
			UnitID:         make([]byte, 32),
			ClientMetadata: &types.ClientMetadata{Timeout: 10},
			SystemID:       []byte{0, 0, 0, 3},
			Attributes:     attrBytes,
		},
	}
	txBytes, _ := cbor.Marshal(tx)
	protoTx := &alphabill.Transaction{Order: txBytes}
	_, err := txClient.ProcessTransaction(ctx, protoTx, grpc.WaitForReady(true))
	require.NoError(t, err)
}
