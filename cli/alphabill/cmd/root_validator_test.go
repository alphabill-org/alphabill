package cmd

import (
	"context"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/stretchr/testify/require"
)

func TestRootValidatorCanBeStarted(t *testing.T) {
	dbDir := t.TempDir()
	defer os.RemoveAll(dbDir)
	conf := validMonolithicRootValidatorConfig(dbDir)
	ctx, _ := async.WithWaitGroup(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := defaultValidatorRunFunc(ctx, conf)
		require.NoError(t, err)
	}()

	cancel()
	wg.Wait() // wait for root validator to close and require statements to execute
}

func TestRootValidatorInvalidRootKey_CannotBeStartedInvalidKeyFile(t *testing.T) {
	dbDir := t.TempDir()
	defer os.RemoveAll(dbDir)
	conf := validMonolithicRootValidatorConfig("")
	conf.KeyFile = "testdata/invalid-root-key.json"
	ctx, _ := async.WithWaitGroup(context.Background())

	err := defaultValidatorRunFunc(ctx, conf)
	require.ErrorContains(t, err, "error root node key not found in genesis file")
}

func TestRootValidatorInvalidRootKey_CannotBeStartedInvalidDBDir(t *testing.T) {
	conf := validMonolithicRootValidatorConfig("/foobar/doesnotexist3454/")
	ctx, _ := async.WithWaitGroup(context.Background())

	err := defaultValidatorRunFunc(ctx, conf)
	require.ErrorContains(t, err, "no such file or directory")
}

func validMonolithicRootValidatorConfig(dbDir string) *validatorConfig {

	conf := &validatorConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		KeyFile:      "testdata/root-key.json",
		GenesisFile:  "testdata/expected/root-genesis.json",
		RootListener: "/ip4/0.0.0.0/tcp/0",
		MaxRequests:  1000,
		StoragePath:  dbDir,
	}
	return conf
}
