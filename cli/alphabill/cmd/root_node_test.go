package cmd

import (
	// "bytes"
	"context"
	// "encoding/json"
	"fmt"
	// "io"
	// "net/http"
	// "net/http/httptest"
	// "os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestRootValidator_OK(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	observe := observability.Default(t)
	obsF := observe.Factory()

	// init money node
	moneyHome := writeShardConf(t, defaultMoneyShardConf)
	cmd := New(obsF)
	cmd.baseCmd.SetArgs([]string{
		"shard-node", "init", "--generate", "--home", moneyHome,
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node 1
	rootHome1 := t.TempDir()
	cmd = New(obsF)
	cmd.baseCmd.SetArgs([]string{
		"root-node", "init", "--generate", "--home", rootHome1,
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node 2
	rootHome2 := t.TempDir()
	cmd = New(obsF)
	cmd.baseCmd.SetArgs([]string{
		"root-node", "init", "--generate", "--home", rootHome2,
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// generate trust base
	cmd = New(obsF)
	cmd.baseCmd.SetArgs([]string{
		"trust-base", "generate",
		"--home", rootHome1,
		"--node-info", filepath.Join(rootHome1, nodeInfoFileName),
	        "--node-info", filepath.Join(rootHome2, nodeInfoFileName),
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// start the root node and expect no errors
	testtime.MustRunInTime(t, 5*time.Second, func() {
		appStoppedWg := sync.WaitGroup{}
		address := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))

		// start the root node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(obsF)
			cmd.baseCmd.SetArgs([]string{
				"root-node", "run",
				"--home", rootHome1,
				"--address", address,
			})
			require.ErrorIs(t, cmd.Execute(ctx), context.Canceled)
		}()

		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func generateSingleNodeSetup(t *testing.T) (string, string) {
	t.Helper()
	rootHomeDir := t.TempDir()
	logF := observability.NewFactory(t)
	// init money node
	moneyHomeDir := writeShardConf(t, defaultMoneyShardConf)
	cmd := New(logF)
	args := "shard-node init -g --home " + moneyHomeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node
	cmd = New(logF)
	args = "root-node init -g --home " + rootHomeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	// generate trust base
	cmd = New(logF)
	args = "trust-base generate --home " + rootHomeDir +
		" --node-info=" + filepath.Join(rootHomeDir, nodeInfoFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	return rootHomeDir, moneyHomeDir
}

func Test_rootNodeConfig_getBootStrapNodes(t *testing.T) {
	t.Run("ok: nil", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes(nil)
		require.NoError(t, err)
		require.NotNil(t, bootNodes)
		require.Empty(t, bootNodes)
	})
	t.Run("ok", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes([]string{"/ip4/127.0.0.1/tcp/1366/p2p/16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz"})
		require.NoError(t, err)
		require.Len(t, bootNodes, 1)
		require.Equal(t, bootNodes[0].ID.String(), "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz")
		require.Len(t, bootNodes[0].Addrs, 1)
		require.Equal(t, bootNodes[0].Addrs[0].String(), "/ip4/127.0.0.1/tcp/1366")
	})
	t.Run("multiple nodes ok", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes([]string{
			"/ip4/127.0.0.1/tcp/1366/p2p/16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz",
			"/ip4/127.0.0.1/tcp/1367/p2p/16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktx",
		})
		require.NoError(t, err)
		require.Len(t, bootNodes, 2)

		require.Equal(t, bootNodes[0].ID.String(), "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz")
		require.Len(t, bootNodes[0].Addrs, 1)
		require.Equal(t, bootNodes[0].Addrs[0].String(), "/ip4/127.0.0.1/tcp/1366")

		require.Equal(t, bootNodes[1].ID.String(), "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktx")
		require.Len(t, bootNodes[1].Addrs, 1)
		require.Equal(t, bootNodes[1].Addrs[0].String(), "/ip4/127.0.0.1/tcp/1367")
	})
}

func TestRootValidator_CannotBeStartedInvalidKeyFile(t *testing.T) {
	rootHome, moneyHome := generateSingleNodeSetup(t)
	wrongKeyConfFile := filepath.Join(moneyHome, keyConfFileName)

	cmd := New(observability.NewFactory(t))
	cmd.baseCmd.SetArgs([]string{
		"root-node", "run",
		"--home", rootHome,
		"--key-conf", wrongKeyConfFile,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), "root node key not found in trust base: node not part of trust base")
}

func TestRootValidator_CannotBeStartedInvalidDBDir(t *testing.T) {
	rootHome, _ := generateSingleNodeSetup(t)

	cmd := New(observability.NewFactory(t))
	invalidStore := "/foobar/doesnotexist3454/"
	cmd.baseCmd.SetArgs([]string{"root-node", "run", "--home", rootHome, "--root-db", invalidStore})

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), fmt.Sprintf("failed to init %q", invalidStore))
}

func getRootValidatorMultiAddress(addressStr string) (multiaddr.Multiaddr, error) {
	rootAddress, err := multiaddr.NewMultiaddr(addressStr)
	if err != nil {
		return nil, err
	}
	return rootAddress, nil
}

type mockNode struct {
	partitionID    types.PartitionID
	peer           *network.Peer
	validatorNodes peer.IDSlice
}

func (mn *mockNode) PartitionID() types.PartitionID {
	return mn.partitionID
}

func (mn *mockNode) ShardID() types.ShardID {
	return types.ShardID{}
}

func (mn *mockNode) Peer() *network.Peer {
	return mn.peer
}

func (mn *mockNode) IsValidator() bool {
	return slices.Contains(mn.validatorNodes, mn.peer.ID())
}

// TODO: re-enable
// func Test_cfgHandler(t *testing.T) {
// 	// helper to set up handler for the case where we expect that the addConfig
// 	// callback is not called (ie handler fails before there is a reason to call it)
// 	setupNoCallbackHandler := func(t *testing.T) (http.HandlerFunc, *httptest.ResponseRecorder) {
// 		return putShardConfigHandler(func(rec *partitions.ValidatorAssignmentRecord) error {
// 				err := fmt.Errorf("unexpected call of addConfig callback with %v", rec)
// 				t.Error(err)
// 				return err
// 			}),
// 			httptest.NewRecorder()
// 	}

// 	t.Run("missing request body", func(t *testing.T) {
// 		hf, w := setupNoCallbackHandler(t)
// 		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", nil))
// 		resp := w.Result()
// 		body, err := io.ReadAll(resp.Body)
// 		require.NoError(t, err)
// 		require.EqualValues(t, http.StatusBadRequest, resp.StatusCode)
// 		require.Equal(t, `parsing var request body: decoding var json: EOF`, string(body))
// 	})

// 	t.Run("invalid request body", func(t *testing.T) {
// 		hf, w := setupNoCallbackHandler(t)
// 		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBufferString("not valid json")))
// 		resp := w.Result()
// 		body, err := io.ReadAll(resp.Body)
// 		require.NoError(t, err)
// 		require.EqualValues(t, http.StatusBadRequest, resp.StatusCode)
// 		require.Equal(t, `parsing var request body: decoding var json: invalid character 'o' in literal null (expecting 'u')`, string(body))
// 	})

// 	// for testing the callback we need valid root genesis - the body parser
// 	// validates the input before calling the callback
// 	genesisFiles := createRootGenesisFiles(t, t.TempDir(), consensusParams{totalNodes: 1})
// 	rootGenesisData, err := os.ReadFile(genesisFiles[0])
// 	require.NoError(t, err)
// 	rg := genesis.RootGenesis{}
// 	require.NoError(t, json.Unmarshal(rootGenesisData, &rg))
// 	rec := partitions.NewVARFromGenesis(rg.Partitions[0])
// 	varBytes, err := json.Marshal(rec)
// 	require.NoError(t, err)

// 	t.Run("config registration fails", func(t *testing.T) {
// 		hf := putShardConfigHandler(func(cfg *partitions.ValidatorAssignmentRecord) error {
// 			return fmt.Errorf("nope, can't add this conf")
// 		})
// 		w := httptest.NewRecorder()
// 		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBuffer(varBytes)))
// 		resp := w.Result()
// 		body, err := io.ReadAll(resp.Body)
// 		require.NoError(t, err)
// 		require.EqualValues(t, http.StatusInternalServerError, resp.StatusCode)
// 		require.Equal(t, `registering configurations: nope, can't add this conf`, string(body))
// 	})

// 	t.Run("success", func(t *testing.T) {
// 		cbCall := false
// 		hf := putShardConfigHandler(func(cfg *partitions.ValidatorAssignmentRecord) error {
// 			cbCall = true
// 			require.Equal(t, rec, cfg)
// 			return nil
// 		})
// 		w := httptest.NewRecorder()
// 		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBuffer(varBytes)))
// 		resp := w.Result()
// 		body, err := io.ReadAll(resp.Body)
// 		require.NoError(t, err)
// 		require.EqualValues(t, http.StatusOK, resp.StatusCode)
// 		require.Empty(t, body)
// 		require.True(t, cbCall, "add configuration callback has not been called")
// 	})
// }
