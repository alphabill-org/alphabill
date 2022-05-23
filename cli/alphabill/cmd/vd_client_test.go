package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	testtime "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"github.com/stretchr/testify/require"
)

func TestVD_UseClientForTx(t *testing.T) {
	homeDirVD := setupTestHomeDir(t, "vd")
	keysFileLocation := path.Join(homeDirVD, keysFile)
	nodeGenesisFileLocation := path.Join(homeDirVD, nodeGenesisFileName)
	partitionGenesisFileLocation := path.Join(homeDirVD, "partition-genesis.json")
	testtime.MustRunInTime(t, 20*time.Second, func() {
		port := "9544"
		listenAddr := ":" + port // listen is on all devices, so it would work in CI inside docker too.
		dialAddr := "localhost:" + port

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
			fmt.Println("Starting VD node")
			cmd := New()
			args := "vd-node --home " + homeDirVD + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err := cmd.addAndExecuteCommand(ctx)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		go func() {
			fmt.Println("Starting VD client 1")
			// Start VD Client
			err := sendTxWithClient(ctx, dialAddr)
			require.NoError(t, err)
		}()

		go func() {
			fmt.Println("Starting VD client 2")
			// failing case, send same stuff once again
			err := sendTxWithClient(ctx, dialAddr)
			// TODO the fact the tx has been rejected is printed in the log, how to verify this in test?
			require.NoError(t, err)
		}()

		// Close the app
		ctxCancel()
		fmt.Println("Sent context cancel")
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func sendTxWithClient(ctx context.Context, dialAddr string) error {
	cmd := New()
	args := "vd register --hash " + "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD" + " -u " + dialAddr
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	return cmd.addAndExecuteCommand(ctx)
}
