package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rpc"
)

func TestRunTokensNode(t *testing.T) {
	homeDir := t.TempDir()
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, utGenesisFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDir, utGenesisStateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "partition-genesis.json")
	trustBaseFileLocation := filepath.Join(homeDir, rootTrustBaseFileName)
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: tokens.DefaultPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2500 * time.Millisecond,
	}
	pdrFilename := filepath.Join(homeDir, "pdr.json")
	require.NoError(t, util.WriteJsonFile(pdrFilename, &pdr))

	testtime.MustRunInTime(t, 5*time.Second, func() {
		ctx, ctxCancel := context.WithCancel(context.Background())
		appStoppedWg := sync.WaitGroup{}
		defer func() {
			ctxCancel()
			appStoppedWg.Wait()
		}()
		logF := testobserve.NewFactory(t)
		// generate node genesis
		cmd := New(logF)
		args := "tokens-genesis --home " + homeDir +
			" --partition-description " + pdrFilename +
			" -o " + nodeGenesisFileLocation +
			" --output-state " + nodeGenesisStateFileLocation +
			" -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(ctx))

		pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)

		// use same keys for signing and authentication.
		rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
		rootPubKeyBytes, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
		require.NoError(t, err)
		rootAuthKey, err := crypto.UnmarshalSecp256k1PublicKey(rootPubKeyBytes)
		require.NoError(t, err)
		rootID, err := peer.IDFromPublicKey(rootAuthKey)
		require.NoError(t, err)
		rootGenesis, partitionGenesisFiles, err := rootgenesis.NewRootGenesis(rootID.String(), rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)
		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)
		trustBase, err := rootGenesis.GenerateTrustBase()
		require.NoError(t, err)
		err = util.WriteJsonFile(trustBaseFileLocation, trustBase)
		require.NoError(t, err)
		rpcServerAddr := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(logF)
			args = "tokens --home " + homeDir +
				" -g " + partitionGenesisFileLocation +
				" -s " + nodeGenesisStateFileLocation +
				" -t " + trustBaseFileLocation +
				" -k " + keysFileLocation +
				" --rpc-server-address " + rpcServerAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()

		// create rpc client
		rpcClient, err := ethrpc.DialContext(ctx, buildRpcUrl(rpcServerAddr))
		require.NoError(t, err)
		defer rpcClient.Close()

		// wait for rpc server to start
		require.Eventually(t, func() bool {
			var res *rpc.NodeInfoResponse
			err := rpcClient.CallContext(ctx, &res, "admin_getNodeInfo")
			return err == nil && res != nil
		}, test.WaitDuration, test.WaitTick)

		// Test
		// green path
		attr := &tokens.DefineNonFungibleTokenAttributes{
			Symbol:                   "Test",
			ParentTypeID:             []byte{0},
			SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
			TokenMintingPredicate:    templates.AlwaysTrueBytes(),
			TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
			DataUpdatePredicate:      templates.AlwaysTrueBytes(),
		}
		attrBytes, err := types.Cbor.Marshal(attr)
		require.NoError(t, err)
		tx := &types.TransactionOrder{
			Version: 1,
			Payload: types.Payload{
				NetworkID:      pdr.NetworkID,
				PartitionID:    pdr.PartitionID,
				Type:           tokens.TransactionTypeDefineNFT,
				Attributes:     attrBytes,
				ClientMetadata: &types.ClientMetadata{Timeout: 10},
			},
		}
		require.NoError(t, tokens.GenerateUnitID(tx, types.ShardID{}, &pdr))
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)
		var res hex.Bytes
		err = rpcClient.CallContext(ctx, &res, "state_sendTransaction", hexutil.Encode(txBytes))
		require.NoError(t, err)
		require.NotNil(t, res)

		// failing case
		var res2 hex.Bytes
		tx.PartitionID = 0x01000000 // incorrect partition id
		txBytes, err = types.Cbor.Marshal(tx)
		require.NoError(t, err)
		err = rpcClient.CallContext(ctx, &res2, "state_sendTransaction", hexutil.Encode(txBytes))
		require.ErrorContains(t, err, "invalid transaction partition identifier")
		require.Nil(t, res2)
	})
}
