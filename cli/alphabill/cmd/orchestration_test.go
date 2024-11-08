package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testutils "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	test "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rpc"
)

func TestRunOrchestrationNode_Ok(t *testing.T) {
	homeDir := t.TempDir()
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, orchestrationGenesisFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDir, orchestrationGenesisStateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "partition-genesis.json")
	trustBaseFileLocation := filepath.Join(homeDir, rootTrustBaseFileName)
	pdr := types.PartitionDescriptionRecord{Version: 1,
		NetworkIdentifier:   5,
		PartitionIdentifier: orchestration.DefaultPartitionID,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           2500 * time.Millisecond,
	}
	pdrFilename := filepath.Join(homeDir, "pdr.json")
	require.NoError(t, util.WriteJsonFile(pdrFilename, &pdr))

	test.MustRunInTime(t, 5*time.Second, func() {
		logF := testobserve.NewFactory(t)

		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New(logF)
		args := "orchestration-genesis --home " + homeDir +
			" --partition-description " + pdrFilename +
			" -o " + nodeGenesisFileLocation +
			" --output-state " + nodeGenesisStateFileLocation +
			" -g -k " + keysFileLocation +
			" --owner-predicate 830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.NoError(t, err)

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)

		// generate root genesis
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		rootPubKeyBytes, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
		require.NoError(t, err)
		rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootPubKeyBytes)
		require.NoError(t, err)
		rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
		require.NoError(t, err)
		rootGenesis, partitionGenesisFiles, err := rootgenesis.NewRootGenesis(rootID.String(), rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)

		// write partition-genesis.json
		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		// write root-trust-base.json
		trustBase, err := rootGenesis.GenerateTrustBase()
		require.NoError(t, err)
		err = util.WriteJsonFile(trustBaseFileLocation, trustBase)
		require.NoError(t, err)

		rpcServerAddress := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(logF)
			args = "orchestration --home " + homeDir +
				" -g " + partitionGenesisFileLocation +
				" -s " + nodeGenesisStateFileLocation +
				" -t " + trustBaseFileLocation +
				" -k " + keysFileLocation +
				" --rpc-server-address " + rpcServerAddress
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()

		t.Log("Started orchestration node and dialing...")

		// create rpc client
		rpcClient, err := ethrpc.DialContext(ctx, buildRpcUrl(rpcServerAddress))
		require.NoError(t, err)
		defer rpcClient.Close()

		// wait for rpc server to start
		require.Eventually(t, func() bool {
			var res *rpc.NodeInfoResponse
			err := rpcClient.CallContext(ctx, &res, "admin_getNodeInfo")
			return err == nil && res != nil
		}, testutils.WaitDuration, testutils.WaitTick)

		// make successful 'addVar' transaction
		attr := &orchestration.AddVarAttributes{}
		attrBytes, err := types.Cbor.Marshal(attr)
		require.NoError(t, err)
		tx := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				Type:           orchestration.TransactionTypeAddVAR,
				UnitID:         orchestration.NewVarID(nil, testutils.RandomBytes(32)),
				ClientMetadata: &types.ClientMetadata{Timeout: 10},
				PartitionID:    orchestration.DefaultPartitionID,
				Attributes:     attrBytes,
			},
		}
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)

		var res hex.Bytes
		err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
		require.NoError(t, err)
		require.NotNil(t, res)

		// Close the app
		ctxCancel()

		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
