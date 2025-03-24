package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/stretchr/testify/require"
)

func TestShardConf_Generate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		logF := testobserve.NewFactory(t)
		homeDir := t.TempDir()

		cmd := New(logF)
		cmd.baseCmd.SetArgs([]string{
			"shard-node", "init", "--home", homeDir, "--gen-keys",
		})
		require.NoError(t, cmd.Execute(context.Background()))

		nodeInfoFile := filepath.Join(homeDir, nodeInfoFileName)

		cmd = New(logF)
		cmd.baseCmd.SetArgs([]string{
			"shard-conf", "generate",
			"--home", homeDir,
			"--node-info", nodeInfoFile,
			"--epoch-number", "0",
			"--epoch-start", "100",
		})
		require.NoError(t, cmd.Execute(context.Background()))

		// verify the resulting file
		rec, err := util.ReadJsonFile(filepath.Join(homeDir, shardConfFileName), &types.PartitionDescriptionRecord{})
		require.NoError(t, err)
		require.Equal(t, types.NetworkID(1), rec.NetworkID)
		require.Equal(t, types.PartitionID(3), rec.PartitionID)
		require.Equal(t, types.ShardID{}, rec.ShardID)
		require.Equal(t, uint64(1), rec.Epoch)
		require.Equal(t, uint64(100), rec.EpochStart)
		require.Len(t, rec.Validators, 1)
	})
}
