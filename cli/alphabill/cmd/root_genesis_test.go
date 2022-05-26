package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateGenesisFiles(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &rootGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionNodeGenesisFiles: []string{"testdata/partition-node-genesis-1.json"},
		Keys: &KeysConfig{
			KeyFilePath: "testdata/root-key.json",
		},
		OutputDir: outputDir,
	}
	err := rootGenesisRunFunc(context.Background(), conf)
	require.NoError(t, err)

	expectedRGFile, _ := ioutil.ReadFile("testdata/expected/root-genesis.json")
	actualRGFile, _ := ioutil.ReadFile(path.Join(outputDir, "root-genesis.json"))
	require.EqualValues(t, expectedRGFile, actualRGFile)

	expectedPGFile1, _ := ioutil.ReadFile("testdata/expected/partition-genesis-1.json")
	actualPGFile1, _ := ioutil.ReadFile(path.Join(outputDir, "partition-genesis-1.json"))
	require.EqualValues(t, expectedPGFile1, actualPGFile1)
}

func TestRootGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestDir(t, alphabillDir)
	cmd := New()
	args := "root-genesis --home " + homeDir + " -p testdata/partition-node-genesis-1.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	s := path.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to read root chain keys from file '%s'", s))
}

func TestRootGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New()
	args := "root-genesis --gen-keys --home " + homeDir + " -p testdata/partition-node-genesis-1.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	kf := path.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
	require.FileExists(t, kf)
}

func TestGenerateGenesisFiles_InvalidPartitionSignature(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &rootGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionNodeGenesisFiles: []string{"testdata/partition-record-1-invalid-sig.json"},
		Keys: &KeysConfig{
			KeyFilePath: "testdata/root-key.json",
		},
		OutputDir: outputDir,
	}
	err := rootGenesisRunFunc(context.Background(), conf)
	require.ErrorContains(t, err, "signature verify failed")
}

func setupTestDir(t *testing.T, dirName string) string {
	outputDir := path.Join(os.TempDir(), dirName)
	_ = os.RemoveAll(outputDir)
	_ = os.MkdirAll(outputDir, 0700) // -rwx------
	t.Cleanup(func() {
		_ = os.RemoveAll(outputDir)
	})
	return outputDir
}
