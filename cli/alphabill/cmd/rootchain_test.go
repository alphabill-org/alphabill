package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testtime "github.com/alphabill-org/alphabill/internal/testutils/time"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
)

func TestRootChainCanBeStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbDir := t.TempDir()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return defaultRootNodeRunFunc(ctx, validMonolithicRootValidatorConfig(t, dbDir))
	})

	g.Go(func() error {
		// give rootchain some time to start up (should try to send message to it to verify it is up!)
		// and then cancel the ctx which should cause it to exit
		time.Sleep(500 * time.Millisecond)
		cancel()
		return nil
	})

	err := g.Wait()
	require.ErrorIs(t, err, context.Canceled)
}

func TestRootValidator_CannotBeStartedInvalidKeyFile(t *testing.T) {
	conf := validMonolithicRootValidatorConfig(t, "")
	conf.KeyFile = "testdata/invalid-root-key.json"

	err := defaultRootNodeRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "root genesis file does not match keys file: signing key not found in genesis file")
}

func TestRootValidator_CannotBeStartedInvalidDBDir(t *testing.T) {
	conf := validMonolithicRootValidatorConfig(t, "/foobar/doesnotexist3454/")
	err := defaultRootNodeRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "root store init failed, open /foobar/doesnotexist3454/rootchain.db: no such file or directory")
}

func TestRootValidator_StorageInitNoDBPath(t *testing.T) {
	db, err := initRootStore("")
	require.Nil(t, db)
	require.ErrorContains(t, err, "persistent storage path not set")
}

func TestRootValidator_DefaultDBPath(t *testing.T) {
	conf := validMonolithicRootValidatorConfig(t, "")
	// if not set it will return a default path
	require.Contains(t, conf.getStoragePath(), filepath.Join(conf.Base.HomeDir, "rootchain"))
}

func validMonolithicRootValidatorConfig(t *testing.T, dbDir string) *rootNodeConfig {
	conf := &rootNodeConfig{
		Base: &baseConfiguration{
			HomeDir: alphabillHomeDir(),
			CfgFile: filepath.Join(alphabillHomeDir(), defaultConfigFile),
			Logger:  logger.New(t),
		},
		KeyFile:           "testdata/root-key.json",
		GenesisFile:       "testdata/expected/root-genesis.json",
		RootListener:      "/ip4/127.0.0.1/tcp/0",
		PartitionListener: "/ip4/127.0.0.1/tcp/0",
		MaxRequests:       1000,
		StoragePath:       dbDir,
	}
	return conf
}

func Test_rootNodeConfig_getBootStrapNodes(t *testing.T) {
	t.Run("ok: nil", func(t *testing.T) {
		cfg := &rootNodeConfig{}
		bootNodes, err := cfg.getBootStrapNodes()
		require.NoError(t, err)
		require.NotNil(t, bootNodes)
		require.Empty(t, bootNodes)
	})
	t.Run("err: invalid parameter", func(t *testing.T) {
		cfg := &rootNodeConfig{
			BootStrapAddresses: "blah",
		}
		bootNodes, err := cfg.getBootStrapNodes()
		require.ErrorContains(t, err, "invalid bootstrap node parameter: blah")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid node description", func(t *testing.T) {
		cfg := &rootNodeConfig{
			BootStrapAddresses: "blah@someip@someif",
		}
		bootNodes, err := cfg.getBootStrapNodes()
		require.ErrorContains(t, err, "invalid bootstrap node parameter: blah@someip@someif")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid node id", func(t *testing.T) {
		cfg := &rootNodeConfig{
			BootStrapAddresses: "blah@someip",
		}
		bootNodes, err := cfg.getBootStrapNodes()
		require.ErrorContains(t, err, "invalid bootstrap node id: blah")
		require.Nil(t, bootNodes)
	})
	t.Run("err: invalid address", func(t *testing.T) {
		cfg := &rootNodeConfig{
			BootStrapAddresses: "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz@someip",
		}
		bootNodes, err := cfg.getBootStrapNodes()
		require.ErrorContains(t, err, "invalid bootstrap node address: someip")
		require.Nil(t, bootNodes)
	})
	t.Run("ok", func(t *testing.T) {
		cfg := &rootNodeConfig{
			BootStrapAddresses: "16Uiu2HAmLEmba2HMEEMe4NYsKnqKToAgi1FueNJaDiAnLeJpKktz@/ip4/127.0.0.1/tcp/1366",
		}
		bootNodes, err := cfg.getBootStrapNodes()
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

func Test_StartMonolithicNode(t *testing.T) {
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	//keysFileLocation := filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	testtime.MustRunInTime(t, 5*time.Second, func() {
		logF := logger.LoggerBuilder(t)
		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())
		partListenddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		cmListenAddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			// generate money node genesis
			cmd := New(logF)
			args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err := cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)
			// create root node genesis with root node
			cmd = New(logF)
			args = "root-genesis new --home " + homeDir +
				" -o " + genesisFileDir +
				" --partition-node-genesis-file=" + nodeGenesisFileLocation +
				" -g"
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)
			// start root node
			cmd = New(logF)
			dbLocation := filepath.Join(homeDir, defaultRootChainDir)
			genesisPath := filepath.Join(homeDir, defaultRootChainDir, rootGenesisFileName)
			keyPath := filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
			args = "root --home " + homeDir + " --db " + dbLocation + " --genesis-file " + genesisPath + " -k " + keyPath + " --partition-listener " + partListenddr + " --root-listener " + cmListenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()
		time.Sleep(2 * time.Second)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}

func Test_Start_2_DRCNodes(t *testing.T) {
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	testtime.MustRunInTime(t, 5*time.Second, func() {
		logF := logger.LoggerBuilder(t)
		appStoppedWg := sync.WaitGroup{}
		ctx, ctxCancel := context.WithCancel(context.Background())
		partListenddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		cmListenAddr := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", net.GetFreeRandomPort(t))
		// start the node in background
		appStoppedWg.Add(1)
		go func() {
			// generate money node genesis
			cmd := New(logF)
			args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err := cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)
			// create root node genesis with root node 1
			genesisFileDirN1 := filepath.Join(homeDir, defaultRootChainDir+"1")
			cmd = New(logF)
			args = "root-genesis new --home " + homeDir +
				" -o " + genesisFileDirN1 +
				" --total-nodes=2" +
				" --partition-node-genesis-file=" + nodeGenesisFileLocation +
				" -g"
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)
			// create root node genesis with root node 2
			genesisFileDirN2 := filepath.Join(homeDir, defaultRootChainDir+"2")
			cmd = New(logF)
			args = "root-genesis new --home " + homeDir +
				" -o " + genesisFileDirN2 +
				" --total-nodes=2" +
				" --partition-node-genesis-file=" + nodeGenesisFileLocation +
				" -g"
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)
			// combine root genesis files
			cmd = New(logF)
			args = "root-genesis combine --home " + homeDir +
				" -o " + homeDir +
				" --root-genesis=" + filepath.Join(genesisFileDirN1, rootGenesisFileName) +
				" --root-genesis=" + filepath.Join(genesisFileDirN2, rootGenesisFileName)
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(context.Background())
			require.NoError(t, err)

			// start root node
			cmd = New(logF)
			dbLocation := filepath.Join(homeDir, defaultRootChainDir+"1")
			genesisPath := filepath.Join(homeDir, rootGenesisFileName)
			keyPath := filepath.Join(homeDir, defaultRootChainDir+"1", defaultKeysFileName)
			args = "root --home " + homeDir + " --db " + dbLocation + " --genesis-file " + genesisPath + " -k " + keyPath + " --partition-listener " + partListenddr + " --root-listener " + cmListenAddr
			cmd.baseCmd.SetArgs(strings.Split(args, " "))
			err = cmd.addAndExecuteCommand(ctx)
			require.ErrorIs(t, err, context.Canceled)
			appStoppedWg.Done()
		}()
		// todo: find a better way to test that root node has started - maybe run money node as well (exits init state)?
		time.Sleep(500 * time.Millisecond)
		// Close the app
		ctxCancel()
		// Wait for test asserts to be completed
		appStoppedWg.Wait()
	})
}
