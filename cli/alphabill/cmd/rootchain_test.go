package cmd

import (
	"context"
	"github.com/stretchr/testify/require"
	"path"
	"testing"
)

func TestRootChainCanBeStarted(t *testing.T) {
	conf := validRootChainConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := defaultRootChainRunFunc(ctx, conf)
	require.NoError(t, err)
}

func TestRootChainInvalidRootKey_CannotBeStarted(t *testing.T) {
	conf := validRootChainConfig()
	conf.KeyFile = "testdata/invalid-root-key.json"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := defaultRootChainRunFunc(ctx, conf)
	require.Errorf(t, err, "invalid genesis")
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
