package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestProgramGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupHome(t, programsDir)
	cmd := New()
	args := "programs-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("load keys %s failed", filepath.Join(homeDir, programsDir, defaultKeysFileName)))
}

func TestProgramGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupHome(t, programsDir)
	cmd := New()
	args := "programs-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, programsDir, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, programsDir, nodeGenesisFileName))
}

func TestProgramGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, programsDir)
	err := os.MkdirAll(filepath.Join(homeDir, programsDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, programsDir, nodeGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New()
	args := "programs-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, programsDir, defaultKeysFileName))
}

func TestProgramGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, programsDir)
	err := os.MkdirAll(filepath.Join(homeDir, programsDir), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, programsDir, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, programsDir, nodeGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New()
	args := "programs-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestProgramGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {

	homeDir := setupTestHomeDir(t, programsDir)
	err := os.MkdirAll(filepath.Join(homeDir, programsDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, programsDir, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, programsDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "programs-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, programsDir, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
}

func TestProgramGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, programsDir)
	err := os.MkdirAll(filepath.Join(homeDir, programsDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, programsDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, programsDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, programsDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "programs-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier)
}

func setupHome(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	err := os.MkdirAll(outputDir, 0700) // -rwx------
	require.NoError(t, err)
	return outputDir
}
