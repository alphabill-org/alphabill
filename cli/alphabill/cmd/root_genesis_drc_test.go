package cmd

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

// Happy path
func TestGenerateDistributedGenesisFiles(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	outputDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=testdata/root1-genesis.json" +
		" --root-genesis=testdata/root2-genesis.json" +
		" --root-genesis=testdata/root3-genesis.json" +
		" --root-genesis=testdata/root4-genesis.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))

	expectedRGFile, _ := os.ReadFile("testdata/expected/distributed-root-genesis.json")
	actualRGFile, _ := os.ReadFile(path.Join(outputDir, "root-genesis.json"))
	require.EqualValues(t, expectedRGFile, actualRGFile)

	expectedPGFile1, _ := os.ReadFile("testdata/expected/distributed-partition-genesis-0.json")
	actualPGFile1, _ := os.ReadFile(path.Join(outputDir, "partition-genesis-0.json"))
	require.EqualValues(t, expectedPGFile1, actualPGFile1)
}

func TestDistributedGenesisFiles_DifferentRootConsensus(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	outputDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=testdata/expected/root-genesis.json" +
		" --root-genesis=testdata/root2-genesis.json" +
		" --root-genesis=testdata/root3-genesis.json" +
		" --root-genesis=testdata/root4-genesis.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), "root genesis merge failed, not compatible root genesis files, consensus is different")
}

func TestDistributedGenesisFiles_DuplicateRootNode(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	outputDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=testdata/root1-genesis.json" +
		" --root-genesis=testdata/root2-genesis.json" +
		" --root-genesis=testdata/root3-genesis.json" +
		" --root-genesis=testdata/root2-genesis.json"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, rootGenesisFileName), &genesis.RootGenesis{})
	require.NoError(t, err)
	// duplicate is ignored
	require.Len(t, rootGenesis.Root.RootValidators, 3)
	require.ErrorContains(t, rootGenesis.Verify(), "root genesis record error: registered root nodes do not match consensus total root nodes")
}
