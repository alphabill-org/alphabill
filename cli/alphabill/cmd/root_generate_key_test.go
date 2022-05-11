package cmd

import (
	"context"
	"path"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

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

	rk, err := util.ReadJsonFile(path.Join(outputDir, rootKeyFileName), &rootKey{})
	require.NoError(t, err)
	require.NotNil(t, rk)

	signer, err := rk.toSigner()
	require.NoError(t, err)
	require.NotNil(t, signer)
}
