package cmd

import (
	"context"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestGenerateGenesisFiles(t *testing.T) {
	outputDir := setupTestDir(t)
	conf := &rootGenesisConfig{
		Root: &rootConfiguration{
			HomeDir:    alphabillHomeDir(),
			CfgFile:    path.Join(alphabillHomeDir(), defaultConfigFile),
			LogCfgFile: defaultLoggerConfigFile,
		},
		PartitionRecordFile: []string{"testdata/partition-record-1.json"},
		KeyFile:             "testdata/root-key.json",
		OutputDir:           outputDir,
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

func setupTestDir(t *testing.T) string {
	outputDir := path.Join(os.TempDir(), "genesis")
	_ = os.RemoveAll(outputDir)
	_ = os.MkdirAll(outputDir, 0700) // -rwx------
	t.Cleanup(func() {
		_ = os.RemoveAll(outputDir)
	})
	return outputDir
}
