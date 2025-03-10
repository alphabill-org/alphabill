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
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/rpc"
)

func TestRunTokensNode(t *testing.T) {
	homeDir := t.TempDir()
	trustBaseFileLocation := filepath.Join(homeDir, trustBaseFileName)
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: tokens.DefaultPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2500 * time.Millisecond,
	}
	pdrFilename := filepath.Join(homeDir, shardConfFileName)
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
		args := "genesis -g --home " + homeDir
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(ctx))

		// TODO: produce VAR/shardConf
		// err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		// require.NoError(t, err)

		_, verifier := testsig.CreateSignerAndVerifier(t)
		trustBase := trustbase.NewTrustBase(t, verifier)

		err := util.WriteJsonFile(trustBaseFileLocation, trustBase)
		require.NoError(t, err)
		rpcServerAddr := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))

		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(logF)
			args = "tokens --home " + homeDir +
				" -t " + trustBaseFileLocation +
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
