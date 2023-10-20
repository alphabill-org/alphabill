package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
)

func TestGenerateGenesisFiles_OK(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir +
		" --partition-node-genesis-file=testdata/partition-node-genesis-0.json" +
		" -k testdata/root-key.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// compare results
	expectedRGFile, _ := os.ReadFile("testdata/expected/root-genesis.json")
	actualRGFile, _ := os.ReadFile(filepath.Join(genesisFileDir, "root-genesis.json"))
	require.EqualValues(t, expectedRGFile, actualRGFile)

	expectedPGFile1, _ := os.ReadFile("testdata/expected/partition-genesis-0.json")
	actualPGFile1, _ := os.ReadFile(filepath.Join(genesisFileDir, "partition-genesis-0.json"))
	require.EqualValues(t, expectedPGFile1, actualPGFile1)
}

func TestRootGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := t.TempDir()
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
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir +
		" --partition-node-genesis-file=testdata/partition-record-0-invalid-sig.json" +
		" -k testdata/root-key.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "signature verification failed")
}
