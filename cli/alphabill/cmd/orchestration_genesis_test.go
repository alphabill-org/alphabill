package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

const (
	orchestrationDir        = "orchestration"
	orchestrationGenesisDir = "orchestration-genesis"
	ownerPredicate          = "830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0"
)

func TestOrchestrationGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", filepath.Join(homeDir, orchestrationDir, defaultKeysFileName)))
}

func TestOrchestrationGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis --gen-keys --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName))
}

func TestOrchestrationGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis --gen-keys --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
}

func TestOrchestrationGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, orchestrationDir, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestOrchestrationGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, orchestrationDir, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, "n1", orchestrationGenesisFileName)
	nodeGenesisStateFile := filepath.Join(homeDir, orchestrationDir, "n1", orchestrationGenesisStateFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
	require.FileExists(t, nodeGenesisStateFile)
}

func TestOrchestrationGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)
	err := os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, orchestrationDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, orchestrationDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, "n1", orchestrationGenesisFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "orchestration-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01020304" + " --home " + homeDir + " --owner-predicate " + ownerPredicate
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.EqualValues(t, 0o1020304, pn.BlockCertificationRequest.SystemIdentifier)
}

func TestOrchestrationGenesis_ParamsCanBeChanged(t *testing.T) {
	homeDir := setupTestHomeDir(t, orchestrationGenesisDir)

	cmd := New(testobserve.NewFactory(t))
	args := fmt.Sprintf("orchestration-genesis --home %s -g --owner-predicate %s", homeDir, ownerPredicate)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	gf := filepath.Join(homeDir, orchestrationPartitionDir, orchestrationGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	var params *genesis.OrchestrationPartitionParams
	err = types.Cbor.Unmarshal(pg.Params, &params)
	require.NoError(t, err)

	expectedOwnerPredicate, err := hex.DecodeString(ownerPredicate)
	require.NoError(t, err)

	require.EqualValues(t, expectedOwnerPredicate, params.OwnerPredicate)
}
