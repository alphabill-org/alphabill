package partition

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func Test_rootNodesSelector(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var nodes peer.IDSlice
		rootNodes, err := rootNodesSelector(nil, nodes, 2)
		require.ErrorContains(t, err, "UC is nil")
		require.Nil(t, rootNodes)
	})
	t.Run("root node list is empty or nil", func(t *testing.T) {
		var nodes peer.IDSlice
		uc := &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
			Version:     1,
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 2)
		require.ErrorContains(t, err, "root node list is empty")
		require.Nil(t, rootNodes)
	})
	t.Run("select 0 nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 1)
		uc := &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
			Version:     1,
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 0)
		require.ErrorContains(t, err, "invalid parameter, number of nodes to select is 0")
		require.Nil(t, rootNodes)
	})
	t.Run("1 root node", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 1)
		uc := &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
			Version:     1,
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 2)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
	})
	t.Run("choose 2 from 3 root nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 3)
		uc := &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
			Version:     1,
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 2)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
		require.Len(t, rootNodes, 2)
	})
	t.Run("choose 4 from 3 root nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 3)
		uc := &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
			Version:     1,
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 4)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
		require.Len(t, rootNodes, 3)
	})
}

func Test_randomNodeSelector(t *testing.T) {
	t.Run("node list is nil", func(t *testing.T) {
		var nodes peer.IDSlice = nil
		rootNodes, err := randomNodeSelector(nodes, 3)
		require.ErrorContains(t, err, "node list is empty")
		require.Nil(t, rootNodes)
	})
	t.Run("node list is empty", func(t *testing.T) {
		var nodes peer.IDSlice
		rootNodes, err := randomNodeSelector(nodes, 3)
		require.ErrorContains(t, err, "node list is empty")
		require.Nil(t, rootNodes)
	})
	t.Run("select 0 nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 1)
		rootNodes, err := randomNodeSelector(nodes, 0)
		require.ErrorContains(t, err, "invalid parameter, number of nodes to select is 0")
		require.Nil(t, rootNodes)
	})
	t.Run("1 root node", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 1)
		rootNodes, err := randomNodeSelector(nodes, 2)
		require.NoError(t, err)
		require.Len(t, rootNodes, 1)
	})
	t.Run("choose 3 from 3 root nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 3)
		rootNodes, err := randomNodeSelector(nodes, 3)
		require.NoError(t, err)
		require.Equal(t, nodes, rootNodes)
		require.Len(t, rootNodes, 3)
	})
	t.Run("choose 3 from 10 root nodes", func(t *testing.T) {
		nodes := test.GeneratePeerIDs(t, 10)
		rootNodes, err := randomNodeSelector(nodes, 3)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
		require.Len(t, rootNodes, 3)
	})
}
