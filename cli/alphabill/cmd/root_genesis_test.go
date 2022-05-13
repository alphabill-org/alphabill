package cmd

import (
	"context"
	"io/ioutil"
	"os"
	"path"
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
		KeyFile:                   "testdata/root-key.json",
		OutputDir:                 outputDir,
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

func TestGenerateGenesisFiles_InvalidPartitionSignature(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &rootGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionNodeGenesisFiles: []string{"testdata/partition-record-1-invalid-sig.json"},
		KeyFile:                   "testdata/root-key.json",
		OutputDir:                 outputDir,
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
