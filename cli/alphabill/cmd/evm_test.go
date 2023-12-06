package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/testutils"
	"github.com/alphabill-org/alphabill/testutils/net"
	testobserve "github.com/alphabill-org/alphabill/testutils/observability"
	"github.com/alphabill-org/alphabill/testutils/sig"
	"github.com/alphabill-org/alphabill/testutils/time"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/util"
	"github.com/alphabill-org/alphabill/wallet"
	evmclient "github.com/alphabill-org/alphabill/wallet/evm/client"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestRunEvmNode(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, evmGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "evm-genesis.json")
	testtime.MustRunInTime(t, 5*time.Second, func() {
		logF := testobserve.NewFactory(t)
		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())

		// generate node genesis
		cmd := New(logF)
		args := "evm-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
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
		_, partitionGenesisFiles, err := rootgenesis.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
		require.NoError(t, err)

		err = util.WriteJsonFile(partitionGenesisFileLocation, partitionGenesisFiles[0])
		require.NoError(t, err)

		listenAddr := fmt.Sprintf("localhost:%d", net.GetFreeRandomPort(t))
		rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootPubKeyBytes)
		require.NoError(t, err)
		rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
		require.NoError(t, err)
		bootNodeStr := fmt.Sprintf("%s@/ip4/127.0.0.1/tcp/26662", rootID.String())
		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			dbLocation := homeDir + "/tx.db"
			cmd = New(logF)
			args = "evm --home " + evmDir + " --tx-db " + dbLocation + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --bootnodes=" + bootNodeStr + " --rest-server-address " + listenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))

			err = cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()
		t.Log("Started evm node")
		// create rest client
		addr, err := url.Parse("http://" + listenAddr)
		require.NoError(t, err)
		restClient := evmclient.New(*addr)
		var info *wallet.InfoResponse
		require.Eventually(t, func() bool {
			info, err = restClient.GetInfo(ctx)
			return err == nil
		}, 2*time.Second, test.WaitTick)
		// Got a session up, so the node has started
		require.NoError(t, err)
		require.Equal(t, hex.EncodeToString(evm.DefaultEvmTxSystemIdentifier), info.SystemID)
		// get node round, but expect failure, since there is no root node running, node is in init state
		require.Eventually(t, func() bool {
			_, err = restClient.GetRoundNumber(ctx)
			return strings.Contains(err.Error(), "initializing")
		}, 2*time.Second, test.WaitTick)
		t.Log("Close evm node")
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func TestRunEvmNode_StartStop(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	keysFileLocation := filepath.Join(homeDir, defaultKeysFileName)
	nodeGenesisFileLocation := filepath.Join(homeDir, evmGenesisFileName)
	partitionGenesisFileLocation := filepath.Join(homeDir, "evm-genesis.json")
	logF := testobserve.NewFactory(t)
	appStoppedWg := sync.WaitGroup{}
	ctx, ctxCancel := context.WithCancel(context.Background())

	// generate node genesis
	cmd := New(logF)
	args := "evm-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + keysFileLocation
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
		args = "evm --home " + evmDir + " --tx-db " + dbLocation + " -g " + partitionGenesisFileLocation + " -k " + keysFileLocation + " --bootnodes=" + bootNodeStr + " --rest-server-address " + listenAddr
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
