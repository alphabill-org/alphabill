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
	logF := testobserve.NewFactory(t)
	homeDir := t.TempDir()

	cmd := New(logF)
	cmd.baseCmd.SetArgs([]string{
		"shard-node", "init", "--home", homeDir, "--generate",
	})
	require.NoError(t, cmd.Execute(context.Background()))

	nodeInfoFile := filepath.Join(homeDir, nodeInfoFileName)

	cmd = New(logF)
	cmd.baseCmd.SetArgs([]string{
		"shard-conf", "generate",
		"--home", homeDir,
		"--node-info", nodeInfoFile,
		"--network-id", "5",
		"--partition-id", "99",
		"--partition-type-id", "1",
		"--epoch", "8",
		"--epoch-start", "100",
	})
	require.NoError(t, cmd.Execute(context.Background()))

	// verify the resulting file
	rec, err := util.ReadJsonFile(filepath.Join(homeDir, "shard-conf-99_8.json"), &types.PartitionDescriptionRecord{})
	require.NoError(t, err)
	require.Equal(t, types.NetworkID(5), rec.NetworkID)
	require.Equal(t, types.PartitionID(99), rec.PartitionID)
	require.Equal(t, types.ShardID{}, rec.ShardID)
	require.Equal(t, uint64(8), rec.Epoch)
	require.Equal(t, uint64(100), rec.EpochStart)
	require.Len(t, rec.Validators, 1)
}
