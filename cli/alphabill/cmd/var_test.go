package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

func Test_VAR_Generate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		nodeGenesis, nodeGenesisFile := newNodeGenesisFile(t, 1, 3, "node-genesis-1.json")
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "var generate --home " + homeDir +
			" --partition-node-genesis-file=" + nodeGenesisFile +
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
		require.Equal(t, nodeGenesis.NodeID, rec.Nodes[0].NodeID)
		require.EqualValues(t, nodeGenesis.SignKey, rec.Nodes[0].SignKey)
		require.EqualValues(t, nodeGenesis.AuthKey, rec.Nodes[0].AuthKey)
	})

	t.Run("multiple node genesis files for different networks nok", func(t *testing.T) {
		_, nodeGenesisFile1 := newNodeGenesisFile(t, 1, 3, "node-genesis-1.json")
		_, nodeGenesisFile2 := newNodeGenesisFile(t, 2, 3, "node-genesis-2.json")
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "var generate --home " + homeDir +
			" --partition-node-genesis-file=" + nodeGenesisFile1 +
			" --partition-node-genesis-file=" + nodeGenesisFile2 +
			" --epoch-number=1" +
			" --round-number=100"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, "node genesis files network ids do not match")
	})

	t.Run("multiple node genesis files for different partitions nok", func(t *testing.T) {
		_, nodeGenesisFile1 := newNodeGenesisFile(t, 1, 3, "node-genesis-1.json")
		_, nodeGenesisFile2 := newNodeGenesisFile(t, 1, 4, "node-genesis-2.json")
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "var generate --home " + homeDir +
			" --partition-node-genesis-file=" + nodeGenesisFile1 +
			" --partition-node-genesis-file=" + nodeGenesisFile2 +
			" --epoch-number=1" +
			" --round-number=100"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, "node genesis files partition ids do not match")
	})
}

func newNodeGenesisFile(t *testing.T, networkID types.NetworkID, partitionID types.PartitionID, outFileName string) (*genesis.PartitionNode, string) {
	testNode := testutils.NewTestNode(t)
	authPubKey := testNode.PeerConf.KeyPair.PublicKey
	sigPubKey, err := testNode.Verifier.MarshalPublicKey()
	require.NoError(t, err)

	nodeGenesis := &genesis.PartitionNode{
		Version: 1,
		NodeID:  testNode.PeerConf.ID.String(),
		SignKey: sigPubKey,
		AuthKey: authPubKey,
		PartitionDescriptionRecord: types.PartitionDescriptionRecord{
			Version:     1,
			NetworkID:   networkID,
			PartitionID: partitionID,
		},
	}
	nodeGenesisFile := filepath.Join(t.TempDir(), outFileName)
	err = util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
	require.NoError(t, err)

	return nodeGenesis, nodeGenesisFile
}
