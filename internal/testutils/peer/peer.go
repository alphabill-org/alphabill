package peer

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/stretchr/testify/require"
)

func CreatePeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{Address: "/ip4/127.0.0.1/tcp/0"}
	peer, err := network.NewPeer(context.Background(), conf)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, peer.Close()) })
	return peer
}
