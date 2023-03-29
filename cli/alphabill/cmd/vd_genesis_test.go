package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	vdDirectory         = "vd"
	vdGenesisDir        = "vd-genesis"
	nodeGenesisFileName = "node-genesis.json"
)

func TestVDGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	cmd := New()
	args := "vd-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", filepath.Join(homeDir, vdDirectory, defaultKeysFileName)))
}

func TestVDGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	cmd := New()
	args := "vd-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, vdDir, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, vdDir, nodeGenesisFileName))
}

func TestVDGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, vdDirectory, nodeGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New()
	args := "vd-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, vdDirectory, defaultKeysFileName))
}

func TestVDGGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, vdDirectory, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, vdDirectory, nodeGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New()
	args := "vd-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestVDGGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {

	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, vdDirectory, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, vdDirectory, "n1", nodeGenesisFileName)

	cmd := New()
	args := "vd-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, vdDirectory, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
}

func TestVDGGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, vdDirectory, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, vdDirectory, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, vdDirectory, "n1", nodeGenesisFileName)

	cmd := New()
	args := "vd-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier)
}

func setupTestHomeDir(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	_ = os.RemoveAll(outputDir)
	_ = os.MkdirAll(outputDir, 0700) // -rwx------
	t.Cleanup(func() {
		_ = os.RemoveAll(outputDir)
	})
	return outputDir
}
