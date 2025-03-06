package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/txsystem/evm"
)

func TestRunEvmNode_StartStop(t *testing.T) {
	homeDir := t.TempDir()
	keysFileLocation := filepath.Join(homeDir, keyConfFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, stateFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDir, stateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "evm-genesis.json")
	trustBaseFileLocation := filepath.Join(homeDir, trustBaseFileName)
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 33,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2500 * time.Millisecond,
	}
	pdrFilename := filepath.Join(homeDir, "pdr.json")
	require.NoError(t, util.WriteJsonFile(pdrFilename, &pdr))

	logF := testobserve.NewFactory(t)
	ctx, ctxCancel := context.WithCancel(context.Background())

	// generate node genesis
	cmd := New(logF)
	args := "evm-genesis --home " + homeDir +
		" --partition-description " + pdrFilename +
		" -o " + nodeGenesisFileLocation +
		" --output-state " + nodeGenesisStateFileLocation +
		" -g -k " + keysFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(ctx))

	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := trustbase.NewTrustBase(t, verifier)
	bootNodeStr := fmt.Sprintf("/ip4/127.0.0.1/tcp/26662/p2p/%s", trustBase.GetRootNodes()[0].NodeID)

	// TODO: this util should probably not be in base
	err := util.WriteJsonFile(trustBaseFileLocation, trustBase)
	require.NoError(t, err)

	listenAddr := fmt.Sprintf("127.0.0.1:%d", net.GetFreeRandomPort(t))
	// start the node in background
	appStoppedWg := sync.WaitGroup{}
	appStoppedWg.Add(1)
	go func() {
		defer appStoppedWg.Done()
		dbLocation := homeDir + "/tx.db"
		cmd = New(logF)
		args = "evm --home " + evm.PartitionType +
			" --tx-db " + dbLocation +
			" -g " + partitionGenesisFileLocation +
			" -s " + nodeGenesisStateFileLocation +
			" -t " + trustBaseFileLocation +
			" -k " + keysFileLocation +
			" --bootnodes=" + bootNodeStr +
			" --rpc-server-address " + listenAddr
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		err = cmd.Execute(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()
	// Close the app, must not crash and should exit normally
	ctxCancel()
	// Wait for test asserts to be completed
	appStoppedWg.Wait()
}
