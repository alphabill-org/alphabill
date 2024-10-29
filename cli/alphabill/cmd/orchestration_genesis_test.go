package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

const (
	orchestrationDir        = "orchestration"
	orchestrationGenesisDir = "orchestration-genesis"
	ownerPredicate          = "830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0"
)

func Test_OrchestrationGenesis(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		NetworkIdentifier:   5,
		PartitionIdentifier: 123,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           5 * time.Second,
	}
	pdrFilename, err := createPDRFile(t.TempDir(), &pdr)
	require.NoError(t, err)
	pdrArgument := " --partition-description " + pdrFilename

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis --home " + homeDir + " --owner-predicate " + ownerPredicate + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, "failed to load keys "+filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis --gen-keys --home " + homeDir + " --owner-predicate " + ownerPredicate + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		require.FileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
		require.FileExists(t, filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName))
	})

	t.Run("DefaultNodeGenesisExists", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700))
		nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName)
		require.NoError(t, util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"}))

		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis --gen-keys --home " + homeDir + " --owner-predicate " + ownerPredicate + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
		require.NoFileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
	})

	t.Run("LoadExistingKeys", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, orchestrationDir), 0700))
		kf := filepath.Join(homeDir, orchestrationDir, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, orchestrationGenesisFileName)
		nodeKeys, err := GenerateKeys()
		require.NoError(t, err)
		require.NoError(t, nodeKeys.WriteTo(kf))

		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis --home " + homeDir + " --owner-predicate " + ownerPredicate + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, kf)
		require.FileExists(t, nodeGenesisFile)
	})

	t.Run("WritesGenesisToSpecifiedOutputLocation", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, orchestrationDir, "n1"), 0700))

		nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, "n1", orchestrationGenesisFileName)
		nodeGenesisStateFile := filepath.Join(homeDir, orchestrationDir, "n1", orchestrationGenesisStateFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + " --owner-predicate " + ownerPredicate + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, orchestrationDir, defaultKeysFileName))
		require.FileExists(t, nodeGenesisFile)
		require.FileExists(t, nodeGenesisStateFile)
	})

	t.Run("partition description is loaded", func(t *testing.T) {
		homeDir := t.TempDir()
		nodeGenesisFile := filepath.Join(homeDir, orchestrationDir, "not-default-name.json")

		pdr := types.PartitionDescriptionRecord{
			NetworkIdentifier:   5,
			PartitionIdentifier: 55,
			TypeIdLen:           4,
			UnitIdLen:           300,
			T2Timeout:           10 * time.Second,
		}
		pdrFile, err := createPDRFile(homeDir, &pdr)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := "orchestration-genesis -g -o " + nodeGenesisFile + " --home " + homeDir + " --owner-predicate " + ownerPredicate + " --partition-description " + pdrFile
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
		require.NoError(t, err)
		require.EqualValues(t, pdr, pn.PartitionDescriptionRecord)
		require.EqualValues(t, pdr.PartitionIdentifier, pn.BlockCertificationRequest.Partition)
	})

	t.Run("ParamsCanBeChanged", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := fmt.Sprintf("orchestration-genesis --home %s -g --owner-predicate %s %s", homeDir, ownerPredicate, pdrArgument)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pg, err := util.ReadJsonFile(filepath.Join(homeDir, orchestrationPartitionDir, orchestrationGenesisFileName), &genesis.PartitionGenesis{})
		require.NoError(t, err)
		require.NotNil(t, pg)

		var params *genesis.OrchestrationPartitionParams
		require.NoError(t, types.Cbor.Unmarshal(pg.Params, &params))

		expectedOwnerPredicate, err := hex.DecodeString(ownerPredicate)
		require.NoError(t, err)
		require.EqualValues(t, expectedOwnerPredicate, params.OwnerPredicate)
	})
}
