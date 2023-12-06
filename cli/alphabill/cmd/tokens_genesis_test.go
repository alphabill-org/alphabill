package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	testobserve "github.com/alphabill-org/alphabill/testutils/observability"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

const (
	utDirectory     = "tokens"
	utGenesisDir    = "ut-genesis"
	genesisFileName = "node-genesis.json"
)

func TestTokensGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", filepath.Join(homeDir, utDirectory, defaultKeysFileName)))
}

func TestTokensGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, utDirectory, genesisFileName))
}

func TestTokensGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, utDirectory, genesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
}

func TestTokensGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, utDirectory, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, utDirectory, genesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestTokensGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, utDirectory, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, utDirectory, "n1", genesisFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
}

func TestTokensGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, utDirectory, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, utDirectory, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, utDirectory, "n1", genesisFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.EqualValues(t, []byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier)
}
