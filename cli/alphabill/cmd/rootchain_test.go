package cmd

import (
	"context"
	gocrypto "crypto"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/rootchain/store"
)

func TestRootChainCanBeStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return defaultRootChainRunFunc(ctx, validRootChainConfig()) })

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

func TestRootChainInvalidRootKey_CannotBeStarted(t *testing.T) {
	conf := validRootChainConfig()
	conf.KeyFile = "testdata/invalid-root-key.json"

	err := defaultRootChainRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "invalid root validator sign key")
}

func validRootChainConfig() *rootChainConfig {
	conf := &rootChainConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		KeyFile:     "testdata/root-key.json",
		GenesisFile: "testdata/expected/root-genesis.json",
		Address:     "/ip4/127.0.0.1/tcp/0",
		T3Timeout:   900,
		MaxRequests: 1000,
		StateStore:  store.NewInMemStateStore(gocrypto.SHA256),
	}
	return conf
}
