package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
)

func TestGetNodeInfo_OK(t *testing.T) {
	node := &MockNode{}
	peerConf := peer.CreatePeerConfiguration(t)
	selfPeer := peer.CreatePeer(t, peerConf)
	log := testlogger.New(t)
	api := NewAdminAPI(node, "money node", selfPeer, log)

	t.Run("ok", func(t *testing.T) {
		r, err := api.GetNodeInfo()
		require.NoError(t, err)
		require.Equal(t, "money node", r.Name)
		require.Equal(t, types.NetworkID(5), r.NetworkID)
		require.Equal(t, types.PartitionID(65536), r.PartitionID)
		require.Equal(t, selfPeer.ID().String(), r.Self.NodeID)
		require.Equal(t, selfPeer.MultiAddresses(), r.Self.Addresses)
		require.Empty(t, r.BootstrapNodes)
		require.Empty(t, r.RootValidators)
		require.Empty(t, r.PartitionValidators)
		require.Empty(t, r.OpenConnections)
	})
}
