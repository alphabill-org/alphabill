package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestRunTokensNode(t *testing.T) {
	homeDir := setupTestHomeDir(t, "tokens")
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, nodeGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "partition-genesis.json")
	testtime.MustRunInTime(t, 5*time.Second, func() {
		ctx, ctxCancel := context.WithCancel(context.Background())
		appStoppedWg := sync.WaitGroup{}
		defer func() {
			ctxCancel()
			appStoppedWg.Wait()
		}()
		// generate node genesis
		cmd := New()
		args := "tokens-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
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
			args = "tokens --home " + homeDir + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()

		// Create the gRPC client
		conn, err := grpc.DialContext(ctx, "localhost"+listenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer conn.Close()
		rpcClient := alphabill.NewAlphabillServiceClient(conn)

		// Test
		// green path
		id := uint256.NewInt(rand.Uint64()).Bytes32()
		tx := &txsystem.Transaction{
			UnitId:                id[:],
			TransactionAttributes: new(anypb.Any),
			ClientMetadata:        &txsystem.ClientMetadata{Timeout: 10},
			SystemId:              tokens.DefaultTokenTxSystemIdentifier,
		}
		require.NoError(t, tx.TransactionAttributes.MarshalFrom(&tokens.CreateNonFungibleTokenTypeAttributes{
			Symbol:                   "Test",
			ParentTypeId:             []byte{0},
			SubTypeCreationPredicate: script.PredicateAlwaysTrue(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
			DataUpdatePredicate:      script.PredicateAlwaysTrue(),
		}))

		_, err = rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.NoError(t, err)

		// failing case
		tx.SystemId = []byte{1, 0, 0, 0} // incorrect system id
		_, err = rpcClient.ProcessTransaction(ctx, tx, grpc.WaitForReady(true))
		require.ErrorContains(t, err, "invalid transaction system identifier")
	})
}
