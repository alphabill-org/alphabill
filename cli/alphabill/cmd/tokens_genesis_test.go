package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

const (
	utDirectory  = "tokens"
	utGenesisDir = "ut-genesis"
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
	require.FileExists(t, filepath.Join(homeDir, utDirectory, utGenesisFileName))
}

func TestTokensGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, utDirectory, utGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
}

func TestTokensGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, utDirectory, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, utDirectory, utGenesisFileName)
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

	nodeGenesisFile := filepath.Join(homeDir, utDirectory, "n1", utGenesisFileName)
	nodeGenesisStateFile := filepath.Join(homeDir, utDirectory, "n1", utGenesisStateFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
	require.FileExists(t, nodeGenesisStateFile)
}

func TestTokensGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, utGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, utDirectory, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, utDirectory, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, utDirectory, "n1", utGenesisFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "tokens-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01020304" + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.EqualValues(t, 0o1020304, pn.BlockCertificationRequest.SystemIdentifier)
}
