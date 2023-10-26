package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
)

func TestGenerateGenesisFiles(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &rootGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionNodeGenesisFiles: []string{"testdata/partition-node-genesis-0.json"},
		Keys: &keysConfig{
			KeyFilePath: "testdata/root-key.json",
		},
		OutputDir:          outputDir,
		TotalNodes:         1,
		BlockRateMs:        900,
		ConsensusTimeoutMs: 10000,
		QuorumThreshold:    1,
	}
	err := rootGenesisRunFunc(conf)
	require.NoError(t, err)

	expectedRGFile, _ := os.ReadFile("testdata/expected/root-genesis.json")
	actualRGFile, _ := os.ReadFile(filepath.Join(outputDir, "root-genesis.json"))
	require.EqualValues(t, expectedRGFile, actualRGFile)

	expectedPGFile1, _ := os.ReadFile("testdata/expected/partition-genesis-0.json")
	actualPGFile1, _ := os.ReadFile(filepath.Join(outputDir, "partition-genesis-0.json"))
	require.EqualValues(t, expectedPGFile1, actualPGFile1)
}

func TestRootGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestDir(t, alphabillDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "root-genesis new --home " + homeDir + " -p testdata/partition-node-genesis-0.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	s := filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to read root chain keys from file '%s'", s))
}

func TestRootGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "root-genesis new --gen-keys --home " + homeDir + " -p testdata/partition-node-genesis-0.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName))
}

func TestGenerateGenesisFiles_InvalidPartitionSignature(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &rootGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    filepath.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionNodeGenesisFiles: []string{"testdata/partition-record-0-invalid-sig.json"},
		Keys: &keysConfig{
			KeyFilePath: "testdata/root-key.json",
		},
		OutputDir: outputDir,
	}
	err := rootGenesisRunFunc(conf)
	require.ErrorContains(t, err, "signature verification failed")
}

func setupTestDir(t *testing.T, dirName string) string {
	outputDir := filepath.Join(os.TempDir(), dirName)
	_ = os.RemoveAll(outputDir)
	_ = os.MkdirAll(outputDir, 0700) // -rwx------
	t.Cleanup(func() {
		_ = os.RemoveAll(outputDir)
	})
	return outputDir
}
