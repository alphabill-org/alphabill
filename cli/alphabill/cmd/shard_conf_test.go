package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

func Test_VAR_Generate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		nodeInfo, nodeInfoFile := newNodeGenesisFile(t, "node-info-1.json")
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "var generate --home " + homeDir +
			" --node-info-file=" + nodeInfoFile +
			" --epoch-number=1" +
			" --round-number=100"

		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		// verify the resulting file
		rec, err := util.ReadJsonFile(filepath.Join(homeDir, "var.json"), &partitions.ValidatorAssignmentRecord{})
		require.NoError(t, err)
		require.Equal(t, types.NetworkID(1), rec.NetworkID)
		require.Equal(t, types.PartitionID(3), rec.PartitionID)
		require.Equal(t, types.ShardID{}, rec.ShardID)
		require.Equal(t, uint64(1), rec.EpochNumber)
		require.Equal(t, uint64(100), rec.RoundNumber)
		require.Len(t, rec.Nodes, 1)
		require.Equal(t, nodeInfo.NodeID, rec.Nodes[0].NodeID)
		require.EqualValues(t, nodeInfo.SigKey, rec.Nodes[0].SigKey)
	})
}

func newNodeGenesisFile(t *testing.T, outFileName string) (*types.NodeInfo, string) {
	testNode := testutils.NewTestNode(t)
	sigKey, err := testNode.Verifier.MarshalPublicKey()
	require.NoError(t, err)

	nodeInfo := &types.NodeInfo{
		NodeID: testNode.PeerConf.ID.String(),
		SigKey: sigKey,
		Stake:  1,
	}
	nodeInfoFile := filepath.Join(t.TempDir(), outFileName)
	err = util.WriteJsonFile(nodeInfoFile, nodeInfo)
	require.NoError(t, err)

	return nodeInfo, nodeInfoFile
}
