package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
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
	conf := &rootNodeConfig{
		Base: &baseConfiguration{
			HomeDir: homeDir,
			CfgFile: filepath.Join(homeDir, defaultConfigFile),
			observe: observability.Default(t),
		},
		StoragePath: "",
	}
	// if not set it will return a default path
	require.Contains(t, conf.getStorageDir(), filepath.Join(conf.Base.HomeDir, "rootchain"))
}

func generateSingleNodeSetup(t *testing.T, homeDir string) (string, string) {
	t.Helper()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName)
	rootDir := filepath.Join(homeDir, defaultRootChainDir)
	logF := observability.NewFactory(t)
	// prepare
	// generate money node genesis
	pdrFilename, err := createPDRFile(homeDir, defaultMoneyPDR)
	require.NoError(t, err)
	cmd := New(logF)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation + " --partition-description " + pdrFilename
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	// create root node genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + rootDir +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
	// create trust base
	cmd = New(logF)
	args = "root-genesis gen-trust-base --home " + homeDir +
		" --root-genesis=" + filepath.Join(rootDir, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	return rootDir, filepath.Join(homeDir, moneyPartitionDir)
}

func Test_rootNodeConfig_getBootStrapNodes(t *testing.T) {
	t.Run("ok: nil", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("")
		require.NoError(t, err)
		require.NotNil(t, bootNodes)
		require.Empty(t, bootNodes)
	})
	t.Run("err: invalid parameter", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("blah")
		require.ErrorContains(t, err, "invalid bootstrap node parameter: blah")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid node description", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("blah@someip@someif")
		require.ErrorContains(t, err, "invalid bootstrap node parameter: blah@someip@someif")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid node id", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("blah@someip")
		require.ErrorContains(t, err, "invalid bootstrap node id: blah")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid address", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz@someip")
		require.ErrorContains(t, err, "invalid bootstrap node address: someip")
		require.Nil(t, bootNodes)
	})
	t.Run("ok", func(t *testing.T) {
		bootNodes, err := getBootStrapNodes("16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz@/ip4/127.0.0.1/tcp/1366")
		require.NoError(t, err)
		require.Len(t, bootNodes, 1)
		require.Equal(t, bootNodes[0].ID.String(), "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz")
		require.Len(t, bootNodes[0].Addrs, 1)
		require.Equal(t, bootNodes[0].Addrs[0].String(), "/ip4/127.0.0.1/tcp/1366")
	})
}

func Test_rootNodeConfig_defaultPath(t *testing.T) {
	t.Run("default keyfile path", func(t *testing.T) {
		cfg := &rootNodeConfig{
			Base: &baseConfiguration{HomeDir: alphabillHomeDir()},
		}
		require.Equal(t, filepath.Join(alphabillHomeDir(), defaultRootChainDir, defaultKeysFileName), cfg.getKeyFilePath())
	})
	t.Run("default genesis path", func(t *testing.T) {
		cfg := &rootNodeConfig{
			Base: &baseConfiguration{HomeDir: alphabillHomeDir()},
		}
		require.Equal(t, filepath.Join(alphabillHomeDir(), defaultRootChainDir, rootGenesisFileName), cfg.getGenesisFilePath())
	})
}

func Test_StartSingleNode(t *testing.T) {
	homeDir := t.TempDir()
	rootDir, nodeDir := generateSingleNodeSetup(t, homeDir)
	observe := observability.Default(t)
	ctx, ctxCancel := context.WithCancel(context.Background())
	testtime.MustRunInTime(t, 500*time.Second, func() {
		address := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		appStoppedWg := sync.WaitGroup{}
		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			// start root node
			cmd := New(observe.Factory())
			dbLocation := filepath.Join(rootDir)
			rootKeyPath := filepath.Join(rootDir, defaultKeysFileName)
			rootGenesis := filepath.Join(rootDir, rootGenesisFileName)
			args := "root --home " + homeDir + " --db=" + dbLocation + " --genesis-file " + rootGenesis + " -k " + rootKeyPath + " --address " + address
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err := cmd.Execute(ctx)
			require.ErrorIs(t, err, context.Canceled)
		}()
		// simulate money partition node sending handshake
		keys, err := LoadKeys(filepath.Join(nodeDir, defaultKeysFileName), false, false)
		require.NoError(t, err)
		partitionGenesis := filepath.Join(homeDir, defaultRootChainDir, "partition-genesis-1.json")
		pg, err := loadPartitionGenesis(partitionGenesis)
		require.NoError(t, err)
		rootValidatorEncryptionKey := pg.RootValidators[0].EncryptionPublicKey
		rootID, rootAddress, err := getRootValidatorIDAndMultiAddress(rootValidatorEncryptionKey, address)
		require.NoError(t, err)
		cfg := &startNodeConfiguration{
			Address: "/ip4/127.0.0.1/tcp/26652",
		}
		moneyPeerCfg, err := loadPeerConfiguration(keys, pg, cfg)
		require.NoError(t, err)
		moneyPeer, err := network.NewPeer(ctx, moneyPeerCfg, observe.Logger(), nil)
		require.NoError(t, err)
		moneyNode := &mockNode{money.DefaultPartitionID, moneyPeer, moneyPeer.Configuration().Validators}
		n, err := network.NewLibP2PValidatorNetwork(
			context.Background(), moneyNode, network.DefaultValidatorNetworkOptions, observe)
		require.NoError(t, err)

		moneyPeer.Network().Peerstore().AddAddr(rootID, rootAddress, peerstore.PermanentAddrTTL)
		require.Eventually(t, func() bool {
			// it is enough that send is success
			err := n.Send(ctx, handshake.Handshake{
				Partition:      money.DefaultPartitionID,
				NodeIdentifier: moneyPeer.ID().String(),
			}, rootID)
			return err == nil
		}, 2*time.Second, test.WaitTick)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func TestRootValidator_CannotBeStartedInvalidKeyFile(t *testing.T) {
	homeDir := t.TempDir()
	rootDir, _ := generateSingleNodeSetup(t, homeDir)
	cmd := New(observability.NewFactory(t))
	dbLocation := filepath.Join(homeDir, defaultRootChainDir)
	rootGenesis := filepath.Join(rootDir, rootGenesisFileName)
	// generate random key file
	randomKeys := filepath.Join(homeDir, "RandomKey", defaultKeysFileName)
	_, err := LoadKeys(randomKeys, true, true)
	require.NoError(t, err)

	args := "root --home " + homeDir + " --db " + dbLocation + " --genesis-file " + rootGenesis + " -k " + randomKeys
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), "root node key not found in genesis: node id/encode key not found in genesis")
}

func TestRootValidator_CannotBeStartedInvalidDBDir(t *testing.T) {
	homeDir := t.TempDir()
	rootDir, _ := generateSingleNodeSetup(t, homeDir)
	cmd := New(observability.NewFactory(t))
	rootGenesis := filepath.Join(rootDir, rootGenesisFileName)
	args := "root --home " + homeDir + " --db=/foobar/doesnotexist3454/" + " --genesis-file " + rootGenesis
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	require.ErrorContains(t, cmd.Execute(ctx), "root store init failed: open /foobar/doesnotexist3454/rootchain.db: no such file or directory")
}

func Test_Start_2_DRCNodes(t *testing.T) {
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName)
	ctx, ctxCancel := context.WithCancel(context.Background())
	observe := observability.Default(t)
	obsF := observe.Factory()
	// prepare genesis files
	// generate money node genesis
	pdrFilename, err := createPDRFile(homeDir, defaultMoneyPDR)
	require.NoError(t, err)
	cmd := New(obsF)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation + " --partition-description " + pdrFilename
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	// create root node genesis with root node 1
	genesisFileDirN1 := filepath.Join(homeDir, defaultRootChainDir+"1")
	cmd = New(obsF)
	args = "root-genesis new --home " + homeDir +
		" -o " + genesisFileDirN1 +
		" --total-nodes=2" +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
	// create root node genesis with root node 2
	genesisFileDirN2 := filepath.Join(homeDir, defaultRootChainDir+"2")
	cmd = New(obsF)
	args = "root-genesis new --home " + homeDir +
		" -o " + genesisFileDirN2 +
		" --total-nodes=2" +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
	// combine root genesis files
	cmd = New(obsF)
	args = "root-genesis combine --home " + homeDir +
		" -o " + homeDir +
		" --root-genesis=" + filepath.Join(genesisFileDirN1, rootGenesisFileName) +
		" --root-genesis=" + filepath.Join(genesisFileDirN2, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
	// create trust base file
	cmd = New(obsF)
	args = "root-genesis gen-trust-base --home " + homeDir +
		" --root-genesis=" + filepath.Join(homeDir, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
	// TODO sign trust base?
	// start a root node and if it receives handshake, then it must be up and running
	testtime.MustRunInTime(t, 5*time.Second, func() {
		appStoppedWg := sync.WaitGroup{}
		address := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		// start the root node in background
		appStoppedWg.Add(1)
		go func() {
			defer appStoppedWg.Done()
			cmd = New(obsF)
			dbLocation := filepath.Join(homeDir, defaultRootChainDir+"1")
			genesisPath := filepath.Join(homeDir, rootGenesisFileName)
			keyPath := filepath.Join(homeDir, defaultRootChainDir+"1", defaultKeysFileName)
			args = "root --home " + homeDir + " --db " + dbLocation + " --genesis-file " + genesisPath + " -k " + keyPath + " --address " + address
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			require.ErrorIs(t, cmd.Execute(ctx), context.Canceled)
		}()
		// simulate money partition node sending handshake
		keys, err := LoadKeys(nodeKeysFileLocation, false, false)
		require.NoError(t, err)
		partitionGenesis := filepath.Join(homeDir, defaultRootChainDir+"1", "partition-genesis-1.json")
		pg, err := loadPartitionGenesis(partitionGenesis)
		require.NoError(t, err)
		rootValidatorEncryptionKey := pg.RootValidators[0].EncryptionPublicKey
		rootID, rootAddress, err := getRootValidatorIDAndMultiAddress(rootValidatorEncryptionKey, address)
		require.NoError(t, err)
		cfg := &startNodeConfiguration{
			Address: "/ip4/127.0.0.1/tcp/26652",
		}
		moneyPeerCfg, err := loadPeerConfiguration(keys, pg, cfg)
		require.NoError(t, err)
		moneyPeer, err := network.NewPeer(ctx, moneyPeerCfg, observe.Logger(), nil)
		require.NoError(t, err)
		moneyNode := &mockNode{money.DefaultPartitionID, moneyPeer, moneyPeer.Configuration().Validators}
		n, err := network.NewLibP2PValidatorNetwork(
			context.Background(), moneyNode, network.DefaultValidatorNetworkOptions, observe)
		require.NoError(t, err)
		moneyPeer.Network().Peerstore().AddAddr(rootID, rootAddress, peerstore.PermanentAddrTTL)
		require.Eventually(t, func() bool {
			// it is enough that send is success
			err := n.Send(ctx, handshake.Handshake{
				Partition:      money.DefaultPartitionID,
				NodeIdentifier: moneyPeer.ID().String(),
			}, rootID)
			return err == nil
		}, 4*time.Second, test.WaitTick)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func getRootValidatorIDAndMultiAddress(rootValidatorEncryptionKey []byte, addressStr string) (peer.ID, multiaddr.Multiaddr, error) {
	rootEncryptionKey, err := crypto.UnmarshalSecp256k1PublicKey(rootValidatorEncryptionKey)
	if err != nil {
		return "", nil, err
	}
	rootID, err := peer.IDFromPublicKey(rootEncryptionKey)
	if err != nil {
		return "", nil, err
	}
	rootAddress, err := multiaddr.NewMultiaddr(addressStr)
	if err != nil {
		return "", nil, err
	}
	return rootID, rootAddress, nil
}

type mockNode struct {
	partitionID    types.PartitionID
	peer           *network.Peer
	validatorNodes peer.IDSlice
}

func (mn *mockNode) PartitionID() types.PartitionID {
	return mn.partitionID
}

func (mn *mockNode) Peer() *network.Peer {
	return mn.peer
}

func (mn *mockNode) IsValidatorNode() bool {
	return slices.Contains(mn.validatorNodes, mn.peer.ID())
}

func Test_cfgHandler(t *testing.T) {

	// helper to set up handler for the case where we expect that the addConfig
	// callback is not called (ie handler fails before there is a reason to call it)
	setupNoCallbackHandler := func(t *testing.T) (http.HandlerFunc, *httptest.ResponseRecorder) {
		return putShardConfigHandler(func(_var *partitions.ValidatorAssignmentRecord) error {
				err := fmt.Errorf("unexpected call of addConfig callback with %v", _var)
				t.Error(err)
				return err
			}),
			httptest.NewRecorder()
	}

	t.Run("missing request body", func(t *testing.T) {
		hf, w := setupNoCallbackHandler(t)
		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", nil))
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.EqualValues(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, `parsing var request body: decoding var json: EOF`, string(body))
	})

	t.Run("invalid request body", func(t *testing.T) {
		hf, w := setupNoCallbackHandler(t)
		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBufferString("not valid json")))
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.EqualValues(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, `parsing var request body: decoding var json: invalid character 'o' in literal null (expecting 'u')`, string(body))
	})

	// for testing the callback we need valid root genesis - the body parser
	// validates the input before calling the callback
	genesisFiles := createRootGenesisFiles(t, t.TempDir(), consensusParams{totalNodes: 1})
	rootGenesisData, err := os.ReadFile(genesisFiles[0])
	require.NoError(t, err)
	rg := genesis.RootGenesis{}
	require.NoError(t, json.Unmarshal(rootGenesisData, &rg))
	_var := partitions.NewVARFromGenesis(rg.Partitions[0])
	varBytes, err := json.Marshal(_var)
	require.NoError(t, err)

	t.Run("config registration fails", func(t *testing.T) {
		hf := putShardConfigHandler(func(cfg *partitions.ValidatorAssignmentRecord) error {
			return fmt.Errorf("nope, can't add this conf")
		})
		w := httptest.NewRecorder()
		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBuffer(varBytes)))
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.EqualValues(t, http.StatusInternalServerError, resp.StatusCode)
		require.Equal(t, `registering configurations: nope, can't add this conf`, string(body))
	})

	t.Run("success", func(t *testing.T) {
		cbCall := false
		hf := putShardConfigHandler(func(cfg *partitions.ValidatorAssignmentRecord) error {
			cbCall = true
			require.Equal(t, _var, cfg)
			return nil
		})
		w := httptest.NewRecorder()
		hf(w, httptest.NewRequest("PUT", "/api/v1/configurations", bytes.NewBuffer(varBytes)))
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.EqualValues(t, http.StatusOK, resp.StatusCode)
		require.Empty(t, body)
		require.True(t, cbCall, "add configuration callback has not been called")
	})
}
