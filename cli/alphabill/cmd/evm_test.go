package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/util"
)

func TestRunEvmNode_StartStop(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, evmGenesisFileName)
	nodeGenesisStateFileLocation := filepath.Join(homeDir, evmGenesisStateFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "evm-genesis.json")
	logF := testobserve.NewFactory(t)
	appStoppedWg := sync.WaitGroup{}
	ctx, ctxCancel := context.WithCancel(context.Background())

	// generate node genesis
	cmd := New(logF)
	args := "evm-genesis --home " + homeDir +
			" -o " + nodeGenesisFileLocation +
			" --output-state " + nodeGenesisStateFileLocation +
			" -g -k " + keysFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	pn, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{})
	require.NoError(t, err)

	// use same keys for signing and communication encryption.
	rootSigner, verifier := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
	require.NoError(t, err)
	rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootPubKeyBytes)
	require.NoError(t, err)
	rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
	require.NoError(t, err)
	bootNodeStr := fmt.Sprintf("%s@/ip4/127.0.0.1/tcp/26662", rootID.String())
	_, partitionGenesisFiles, err := rootgenesis.NewRootGenesis(rootID.String(), rootSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)

	err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
	require.NoError(t, err)

	listenAddr := fmt.Sprintf("localhost:%d", net.GetFreeRandomPort(t))
	// start the node in background
	appStoppedWg.Add(1)
	go func() {
		dbLocation := homeDir + "/tx.db"
		cmd = New(logF)
		args = "evm --home " + evmDir +
			" --tx-db " + dbLocation +
			" -g " + partitionGenesisFileLocation +
			" -s " + nodeGenesisStateFileLocation +
			" -k " + keysFileLocation +
			" --bootnodes=" + bootNodeStr +
			" --rpc-server-address " + listenAddr
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		err = cmd.Execute(ctx)
		require.ErrorIs(t, err, context.Canceled)
		appStoppedWg.Done()
	}()
	// Close the app, must not crash and should exit normally
	ctxCancel()
	// Wait for test asserts to be completed
	appStoppedWg.Wait()
}
