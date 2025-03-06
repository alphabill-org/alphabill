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

func TestRootValidator_StorageInitNoDBPath(t *testing.T) {
	db, err := initRootStore("")
	require.Nil(t, db)
	require.ErrorContains(t, err, "persistent storage path not set")
}

func TestRootValidator_DefaultDBPath(t *testing.T) {
	homeDir := t.TempDir()
	conf := &rootNodeRunFlags{
		Base: &baseFlags{
			HomeDir: homeDir,
			CfgFile: filepath.Join(homeDir, defaultConfigFile),
			observe: observability.Default(t),
		},
		RootStorePath: "",
	}
	// if not set it will return a default path
	require.Contains(t, conf.getStorageDir(), filepath.Join(conf.Base.HomeDir, "rootchain"))
}

func TestRootValidator_OK(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	observe := observability.Default(t)
	obsF := observe.Factory()

	// init money node
	moneyHome := createPDRFile(t, defaultMoneyPDR)
	cmd := New(obsF)
	args := "node init -g --home " + moneyHome
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node 1
	rootHome1 := t.TempDir()
	cmd = New(obsF)
	args = "node init -g --home " + rootHome1
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node 2
	rootHome2 := t.TempDir()
	cmd = New(obsF)
	args = "node init -g --home " + rootHome2
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// generate trust base
	cmd = New(obsF)
	args = "trust-base generate --home " + rootHome1 +
		" --node-info-file=" + filepath.Join(rootHome1, nodeInfoFileName) +
	        " --node-info-file=" + filepath.Join(rootHome2, nodeInfoFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// sign trust base
	// TODO: TB signatures are currently not verified, should?
	// cmd = New(obsF)
	// args = "trust-base sign --home " + rootHome1
	// cmd.baseCmd.SetArgs(strings.Split(args, " "))
	// require.NoError(t, cmd.Execute(context.Background()))

	// start the root node and expect no errors
	testtime.MustRunInTime(t, 5*time.Second, func() {
		appStoppedWg := sync.WaitGroup{}
		address := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))

		// start the root node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(obsF)
			args = "root --home " + rootHome1 + " --address " + address
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
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
	moneyHomeDir := createPDRFile(t, defaultMoneyPDR)
	cmd := New(logF)
	args := "node init -g --home " + moneyHomeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	// init root node
	cmd = New(logF)
	args = "node init -g --home " + rootHomeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	// generate trust base
	cmd = New(logF)
	args = "trust-base generate --home " + rootHomeDir +
		" --node-info-file=" + filepath.Join(rootHomeDir, nodeInfoFileName)
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

//TODO: re-enable
// func Test_rootNodeConfig_defaultPath(t *testing.T) {
// 	t.Run("default keyfile path", func(t *testing.T) {
// 		cfg := &rootNodeConfig{
// 			Base: &baseConfiguration{HomeDir: alphabillHomeDir()},
// 		}
// 		require.Equal(t, filepath.Join(alphabillHomeDir(), defaultRootChainDir, defaultKeysFileName), cfg.getKeyFilePath())
// 	})
// 	t.Run("default genesis path", func(t *testing.T) {
// 		cfg := &rootNodeConfig{
// 			Base: &baseConfiguration{HomeDir: alphabillHomeDir()},
// 		}
// 		require.Equal(t, filepath.Join(alphabillHomeDir(), defaultRootChainDir, rootGenesisFileName), cfg.getGenesisFilePath())
// 	})
// }

func TestRootValidator_CannotBeStartedInvalidKeyFile(t *testing.T) {
	homeDir := t.TempDir()
	rootDir, _ := generateSingleNodeSetup(t)
	cmd := New(observability.NewFactory(t))
	trustBase := filepath.Join(rootDir, trustBaseFileName)
	// generate random key file
	randomKeys := filepath.Join(homeDir, "RandomKey", keyConfFileName)
	_, err := LoadKeys(randomKeys, true, true)
	require.NoError(t, err)

	args := "root --home " + homeDir + " --trust-base-file " + trustBase + " -k " + randomKeys
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), "root node key not found in genesis: node id/encode key not found in genesis")
}

func TestRootValidator_CannotBeStartedInvalidDBDir(t *testing.T) {
	homeDir := t.TempDir()
	rootDir, _ := generateSingleNodeSetup(t)
	cmd := New(observability.NewFactory(t))
	trustBase := filepath.Join(rootDir, trustBaseFileName)
	args := "root --home " + homeDir + " --db=/foobar/doesnotexist3454/" + " --trust-base-file " + trustBase
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), "root store init failed: open /foobar/doesnotexist3454/rootchain.db: no such file or directory")
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
