package cmd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestRootChainCanBeStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbDir := t.TempDir()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return defaultRootNodeRunFunc(ctx, validMonolithicRootValidatorConfig(dbDir)) })

	g.Go(func() error {
		// give rootchain some time to start up (should try to sens message it to verify it is up!)
		// and then cancel the ctx which should cause it to exit
		time.Sleep(500 * time.Millisecond)
		cancel()
		return nil
	})

	err := g.Wait()
	require.ErrorIs(t, err, context.Canceled)
}

func TestRootValidator_CannotBeStartedInvalidKeyFile(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("")
	conf.KeyFile = "testdata/invalid-root-key.json"

	err := defaultRootNodeRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "error root node key not found in genesis file")
}

func TestRootValidator_CannotBeStartedInvalidDBDir(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("/foobar/doesnotexist3454/")
	err := defaultRootNodeRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "root store init failed, open /foobar/doesnotexist3454/rootchain.db: no such file or directory")
}

func TestRootValidator_StorageInitNoDBPath(t *testing.T) {
	db, err := initRootStore("")
	require.Nil(t, db)
	require.ErrorContains(t, err, "persistent storage path not set")
}

func TestRootValidator_DefaultDBPath(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("")
	// if not set it will return a default path
	require.Contains(t, conf.getStoragePath(), filepath.Join(conf.Base.HomeDir, "rootchain"))
}

func validMonolithicRootValidatorConfig(dbDir string) *rootNodeConfig {
	conf := &rootNodeConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		KeyFile:      "testdata/root-key.json",
		GenesisFile:  "testdata/expected/root-genesis.json",
		RootListener: "/ip4/127.0.0.1/tcp/0",
		MaxRequests:  1000,
		StoragePath:  dbDir,
	}
	return conf
}
