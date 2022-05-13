package cmd

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateKeys(t *testing.T) {
	outputDir := setupTestDir(t, "root-keys")
	conf := &generateKeyConfig{
		&rootGenesisConfig{
			Base: &baseConfiguration{
				HomeDir:    alphabillHomeDir(),
				CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
				LogCfgFile: defaultLoggerConfigFile,
			},
		}, outputDir,
	}
	err := generateKeysRunFunc(context.Background(), conf)
	require.NoError(t, err)

	keys, err := LoadKeys(path.Join(outputDir, rootKeyFileName), false)
	require.NoError(t, err)
	require.NotNil(t, keys)

	signer := keys.SigningPrivateKey
	require.NotNil(t, signer)
}
