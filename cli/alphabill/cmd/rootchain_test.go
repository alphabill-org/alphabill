package cmd

import (
	"context"
	"path"
	"sync"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"github.com/stretchr/testify/require"
)

func TestRootChainCanBeStarted(t *testing.T) {
	conf := validRootChainConfig()
	ctx, _ := async.WithWaitGroup(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := defaultRootChainRunFunc(ctx, conf)
		require.NoError(t, err)
		wg.Done()
	}()

	cancel()
	wg.Wait() // wait for rootchain to close and require statements to execute
}

func TestRootChainInvalidRootKey_CannotBeStarted(t *testing.T) {
	conf := validRootChainConfig()
	conf.KeyFile = "testdata/invalid-root-key.json"
	ctx, _ := async.WithWaitGroup(context.Background())

	err := defaultRootChainRunFunc(ctx, conf)
	require.ErrorContains(t, err, "invalid genesis")
}

func validRootChainConfig() *rootChainConfig {
	conf := &rootChainConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		KeyFile:     "testdata/root-key.json",
		GenesisFile: "testdata/expected/root-genesis.json",
		Address:     "/ip4/0.0.0.0/tcp/0",
		T3Timeout:   900,
		MaxRequests: 1000,
	}
	return conf
}
