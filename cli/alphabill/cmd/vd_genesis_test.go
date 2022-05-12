package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

const (
	vdDirectory         = "vd"
	vdGenesisDir        = "vd-genesis"
	keysFile            = "keys.json"
	nodeGenesisFileName = "node-genesis.json"
)

func TestVDGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	cmd := New()
	args := "vd-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", path.Join(homeDir, vdDirectory, keysFile)))
}

func TestVDGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	cmd := New()
	args := "vd-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	kf := path.Join(homeDir, vdDir, keysFile)
	gf := path.Join(homeDir, vdDir, nodeGenesisFileName)
	require.FileExists(t, kf)
	require.FileExists(t, gf)
}

func TestVDGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(path.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	nodeGenesisFile := path.Join(homeDir, vdDirectory, nodeGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New()
	args := "vd-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	kf := path.Join(homeDir, vdDirectory, keysFile)
	require.NoFileExists(t, kf)
}

func TestVDGGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(path.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)
	kf := path.Join(homeDir, vdDirectory, keysFile)
	nodeGenesisFile := path.Join(homeDir, vdDirectory, nodeGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New()
	args := "vd-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestVDGGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {

	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(path.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, vdDirectory, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, vdDirectory, keysFile)

	nodeGenesisFile := path.Join(homeDir, vdDirectory, "n1", nodeGenesisFileName)

	cmd := New()
	args := "vd-genesis --force-key-gen -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestVDGGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, vdGenesisDir)
	err := os.MkdirAll(path.Join(homeDir, vdDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, vdDirectory, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, vdDirectory, "n1", keysFile)
	nodeGenesisFile := path.Join(homeDir, vdDirectory, "n1", nodeGenesisFileName)

	cmd := New()
	args := "vd-genesis -f -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 1}, pn.P1Request.SystemIdentifier)
}

func setupTestHomeDir(t *testing.T, dir string) string {
	outputDir := path.Join(os.TempDir(), dir)
	_ = os.RemoveAll(outputDir)
	_ = os.MkdirAll(outputDir, 0700) // -rwx------
	t.Cleanup(func() {
		_ = os.RemoveAll(outputDir)
	})
	return outputDir
}
