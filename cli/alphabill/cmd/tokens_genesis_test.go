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

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

func Test_TokensGenesis(t *testing.T) {
	// create partition description file to be shared in all the tests
	pdr := types.PartitionDescriptionRecord{
		Version:             1,
		NetworkIdentifier:   5,
		PartitionIdentifier: 2,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           3 * time.Second,
	}
	pdrFilename, err := createPDRFile(t.TempDir(), &pdr)
	require.NoError(t, err)
	pdrArgument := " --partition-description " + pdrFilename
	const utDirectory = "tokens"

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "tokens-genesis --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", filepath.Join(homeDir, utDirectory, defaultKeysFileName)))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "tokens-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		require.FileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
		require.FileExists(t, filepath.Join(homeDir, utDirectory, utGenesisFileName))
	})

	t.Run("LoadExistingKeys", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700))
		kf := filepath.Join(homeDir, utDirectory, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, utDirectory, utGenesisFileName)
		nodeKeys, err := GenerateKeys()
		require.NoError(t, err)
		require.NoError(t, nodeKeys.WriteTo(kf))

		cmd := New(testobserve.NewFactory(t))
		args := "tokens-genesis --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.NoError(t, err)

		require.FileExists(t, kf)
		require.FileExists(t, nodeGenesisFile)
	})

	t.Run("WritesGenesisToSpecifiedOutputLocation", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, utDirectory, "n1"), 0700))

		nodeGenesisFile := filepath.Join(homeDir, utDirectory, "n1", utGenesisFileName)
		nodeGenesisStateFile := filepath.Join(homeDir, utDirectory, "n1", utGenesisStateFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "tokens-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
		require.FileExists(t, nodeGenesisFile)
		require.FileExists(t, nodeGenesisStateFile)
	})

	t.Run("PartitionParams", func(t *testing.T) {
		homeDir := t.TempDir()
		adminOwnerPredicate := "830041025820f34a250bf4f2d3a432a43381cecc4ab071224d9ceccb6277b5779b937f59055f"

		cmd := New(testobserve.NewFactory(t))
		args := fmt.Sprintf("tokens-genesis -g --home %s %s --admin-owner-predicate %s --feeless-mode true", homeDir, pdrArgument, adminOwnerPredicate)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		nodeGenesisFile := filepath.Join(homeDir, utDirectory, utGenesisFileName)
		pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)
		var params *genesis.TokensPartitionParams
		require.NoError(t, types.Cbor.Unmarshal(pn.Params, &params))
		require.NotNil(t, params)
		require.Equal(t, adminOwnerPredicate, hex.EncodeToString(params.AdminOwnerPredicate))
		require.True(t, params.FeelessMode)
	})

	t.Run("DefaultNodeGenesisExists", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, utDirectory), 0700))
		nodeGenesisFile := filepath.Join(homeDir, utDirectory, utGenesisFileName)
		require.NoError(t, util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1, NodeIdentifier: "1"}))

		cmd := New(testobserve.NewFactory(t))
		args := "tokens-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
		require.NoFileExists(t, filepath.Join(homeDir, utDirectory, defaultKeysFileName))
	})
}
