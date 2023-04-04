package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestRootChainCanBeStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbDir := t.TempDir()
	defer func() { require.NoError(t, os.RemoveAll(dbDir)) }()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return defaultValidatorRunFunc(ctx, validMonolithicRootValidatorConfig(dbDir)) })

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

func TestRootValidatorInvalidRootKey_CannotBeStartedInvalidKeyFile(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("")
	conf.KeyFile = "testdata/invalid-root-key.json"

	err := defaultValidatorRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "error root node key not found in genesis file")
}

func TestRootValidatorInvalidRootKey_CannotBeStartedInvalidDBDir(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("/foobar/doesnotexist3454/")
	err := defaultValidatorRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "no such file or directory")
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
