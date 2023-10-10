package peer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
)

func CreatePeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{Address: "/ip4/127.0.0.1/tcp/0"}
	peer, err := network.NewPeer(context.Background(), conf, logger.New(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, peer.Close()) })
	return peer
}
