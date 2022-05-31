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
	keysFileLocation := path.Join(homeDirVD, defaultKeysFileName)
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
		args := "vd-genesis --home " + homeDirVD + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
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
			args := "vd --home " + homeDirVD + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err := cmd.addAndExecuteCommand(ctx)
			require.NoError(t, err)
			appStoppedWg.Done()
		}()

		fmt.Println("Starting VD clients")
		// Start VD Client
		require.NoError(t, sendTxWithClient(ctx, dialAddr))

		// failing case, send same stuff once again
		err = sendTxWithClient(ctx, dialAddr)
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

		// list non-empty blocks
		cmd = New()
		cmd.baseCmd.SetArgs(strings.Split("vd-client list-blocks --wait -u "+dialAddr, " "))
		require.NoError(t, cmd.addAndExecuteCommand(ctx))

		// Close the app
		ctxCancel()
		fmt.Println("Sent context cancel")
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func sendTxWithClient(ctx context.Context, dialAddr string) error {
	cmd := New()
	args := "vd-client register --hash " + "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD" + " -u " + dialAddr + " --wait"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	return cmd.addAndExecuteCommand(ctx)
}
