package partition

import (
	"crypto/rand"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/types"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func Test_rootNodesSelector(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var nodes peer.IDSlice
		rootNodes, err := rootNodesSelector(nil, nodes, defaultNofRootNodes)
		require.ErrorContains(t, err, "UC is nil")
		require.Nil(t, rootNodes)
	})
	t.Run("UC is nil", func(t *testing.T) {
		var nodes peer.IDSlice
		uc := &types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, defaultNofRootNodes)
		require.ErrorContains(t, err, "root node list is empty")
		require.Nil(t, rootNodes)
	})
	t.Run("1 root node", func(t *testing.T) {
		nodes := generatePeerIDs(t, 1)
		uc := &types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, defaultNofRootNodes)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
	})
	t.Run("choose 2 from 3 root nodes", func(t *testing.T) {
		nodes := generatePeerIDs(t, 3)
		uc := &types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, defaultNofRootNodes)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
		require.Len(t, rootNodes, 2)
	})
	t.Run("choose 4 from 3 root nodes", func(t *testing.T) {
		nodes := generatePeerIDs(t, 3)
		uc := &types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber: 1,
		}}
		rootNodes, err := rootNodesSelector(uc, nodes, 4)
		require.NoError(t, err)
		require.NotNil(t, rootNodes)
		require.Len(t, rootNodes, 3)
	})
}

func generatePeerIDs(t *testing.T, count int) peer.IDSlice {
	t.Helper()
	peers := make(peer.IDSlice, count)
	for i := range peers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		peers[i], err = network.NodeIDFromPublicKeyBytes(pubKeyBytes)
		require.NoError(t, err)
	}
	sort.Sort(peers)
	return peers
}
