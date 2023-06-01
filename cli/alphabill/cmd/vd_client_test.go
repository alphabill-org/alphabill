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
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestVD_UseClientForTx(t *testing.T) {
	homeDirVD := setupTestHomeDir(t, "vd")
	keysFileLocation := filepath.Join(homeDirVD, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDirVD, nodeGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDirVD, "partition-genesis.json")
	testtime.MustRunInTime(t, 20*time.Second, func() {
		freePort, err := net.GetFreePort()
		require.NoError(t, err)
		listenAddr := fmt.Sprintf(":%v", freePort) // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := fmt.Sprintf("localhost:%v", freePort)

		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New()
		args := "vd-genesis --home " + homeDirVD + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.addAndExecuteCommand(context.Background())
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
			fmt.Println("Starting VD node")
			cmd := New()
			args := "vd --home " + homeDirVD + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err := cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()

		// Server startup takes time, somehow check if server is ready?
		time.Sleep(1 * time.Second)

		fmt.Println("Starting VD clients")

		// Create temporary wallet
		walletHomeDir := t.TempDir()
		require.NoError(t, createTempWallet(ctx, walletHomeDir))
		// Start VD Client
		require.NoError(t, sendTxWithClient(ctx, dialAddr, walletHomeDir))

		// failing case, send same stuff once again
		err = sendTxWithClient(ctx, dialAddr, walletHomeDir)
		// There are two cases, then second 'register tx' gets rejected:
		if err != nil {
			// first, when both txs end up in the same block, this error is propagated here:
			fmt.Println("second tx rejected from the buffer")
			require.ErrorContains(t, err, "tx already in tx buffer")
		} else {
			// second, if the first tx has been processed, the second tx is rejected,
			// but the error is only printed to the log and not propagated back here (TODO)
			fmt.Println("second tx rejected, but error not propagated")
		}

		// Close the app
		ctxCancel()
		fmt.Println("Sent context cancel")
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func createTempWallet(ctx context.Context, walletHomeDir string) error {
	cmd := New()
	args := "wallet create -l " + walletHomeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	return cmd.addAndExecuteCommand(ctx)
}

func sendTxWithClient(ctx context.Context, dialAddr string, walletHomeDir string) error {
	cmd := New()
	args := "wallet vd register " +
		" --hash 0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD" +
		" -u " + dialAddr +
		" -l " + walletHomeDir +
		" --wait"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	return cmd.addAndExecuteCommand(ctx)
}
