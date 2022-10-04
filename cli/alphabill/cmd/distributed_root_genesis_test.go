package cmd

import (
	"context"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"path"
	"testing"
)

// Happy path
func TestGenerateDistributedGenesisFiles(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &distributedGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		OutputDir: outputDir,
		RootGenesisFiles: []string{
			"testdata/root1-genesis.json",
			"testdata/root2-genesis.json",
			"testdata/root3-genesis.json",
			"testdata/root4-genesis.json"},
	}
	err := distributedRootGenesisRunFunc(context.Background(), conf)
	require.NoError(t, err)
	expectedRGFile, _ := ioutil.ReadFile("testdata/expected/distributed-root-genesis.json")
	actualRGFile, _ := ioutil.ReadFile(path.Join(outputDir, "root-genesis.json"))
	require.EqualValues(t, expectedRGFile, actualRGFile)

	expectedPGFile1, _ := ioutil.ReadFile("testdata/expected/distributed-partition-genesis-1.json")
	actualPGFile1, _ := ioutil.ReadFile(path.Join(outputDir, "partition-genesis-1.json"))
	require.EqualValues(t, expectedPGFile1, actualPGFile1)
}

func TestDistributedGenesisFiles_DifferentRootConsensus(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &distributedGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		OutputDir: outputDir,
		RootGenesisFiles: []string{
			"testdata/expected/root-genesis.json", // This is a monolithic root node genesis file
			"testdata/root2-genesis.json",
			"testdata/root3-genesis.json",
			"testdata/root4-genesis.json"},
	}
	err := distributedRootGenesisRunFunc(context.Background(), conf)
	require.Error(t, err)
}

func TestDistributedGenesisFiles_DuplicateRootNode(t *testing.T) {
	outputDir := setupTestDir(t, "genesis")
	conf := &distributedGenesisConfig{
		Base: &baseConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		OutputDir: outputDir,
		RootGenesisFiles: []string{
			"testdata/root1-genesis.json", // This is a monolithic root node genesis file
			"testdata/root2-genesis.json",
			"testdata/root3-genesis.json",
			"testdata/root2-genesis.json"},
	}
	err := distributedRootGenesisRunFunc(context.Background(), conf)
	require.Error(t, err)
}
