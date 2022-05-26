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

const alphabillDir = "ab"
const moneyGenesisDir = "money"

func TestMoneyGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestDir(t, alphabillDir)
	cmd := New()
	args := "money-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	s := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
}

func TestMoneyGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New()
	args := "money-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	gf := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	require.FileExists(t, kf)
	require.FileExists(t, gf)
}

func TestMoneyGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New()
	args := "money-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.NoFileExists(t, kf)
}

func TestMoneyGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)
	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New()
	args := "money-genesis --force-key-gen --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, alphabillDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)

	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "money-genesis --force-key-gen -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, "n1", defaultKeysFileName)
	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "money-genesis -f -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 1}, pn.P1Request.SystemIdentifier)
}
