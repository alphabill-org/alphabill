package cmd

import (
	"context"
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateGenesisFiles(t *testing.T) {
	outputDir := setupTestDir(t)
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
	outputDir := setupTestDir(t)
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
	require.Errorf(t, err, "signature verify failed")
}

func setupTestDir(t *testing.T) string {
	return setupTestHomeDir(t, "genesis")
}
