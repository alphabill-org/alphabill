package cmd

import (
	"context"
	"path"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/async"
	"github.com/stretchr/testify/require"
)

func TestRootValidatorCanBeStarted(t *testing.T) {
	conf := validMonolithicRootValidatorConfig()
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

func TestRootValidatorInvalidRootKey_CannotBeStarted(t *testing.T) {
	conf := validMonolithicRootValidatorConfig()
	conf.KeyFile = "testdata/invalid-root-key.json"
	ctx, _ := async.WithWaitGroup(context.Background())

	err := defaultValidatorRunFunc(ctx, conf)
	require.ErrorContains(t, err, "error root node key not found in genesis file")
}

func validMonolithicRootValidatorConfig() *validatorConfig {
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
		StoragePath:  "",
	}
	return conf
}
