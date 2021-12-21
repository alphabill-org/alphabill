package network

import (
	"context"
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/testnetwork"
	golog "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	lpdht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/stretchr/testify/require"
)

func TestNewDHT(t *testing.T) {
	golog.SetAllLoggers(golog.LevelDebug) // change this to Debug if libp2p logs are needed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dht, err := createDHT(t, ctx)
	require.NoError(t, err)
	require.Equal(t, lpdht.ModeServer, dht.Mode())

	bootstrapPeers := lpdht.BootstrapPeers(peer.AddrInfo{ID: dht.Host().ID(), Addrs: dht.Host().Addrs()})
	dht2, err := createDHT(t, ctx, bootstrapPeers)
	require.NoError(t, err)
	require.Equal(t, lpdht.ModeServer, dht.Mode())

	require.Eventually(t, func() bool { return dht.RoutingTable().Size() == 1 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht2.RoutingTable().Size() == 1 }, test.WaitDuration, test.WaitTick)

	dht3, err := createDHT(t, ctx, bootstrapPeers)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return dht.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht2.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht3.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)

	require.Eventually(t, func() bool { return dht.RoutingTable().Find(dht2.PeerID()) != "" }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht.RoutingTable().Find(dht3.PeerID()) != "" }, test.WaitDuration, test.WaitTick)

	require.Eventually(t, func() bool { return dht2.RoutingTable().Find(dht.PeerID()) != "" }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht2.RoutingTable().Find(dht3.PeerID()) != "" }, test.WaitDuration, test.WaitTick)

	require.Eventually(t, func() bool { return dht3.RoutingTable().Find(dht.PeerID()) != "" }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return dht3.RoutingTable().Find(dht2.PeerID()) != "" }, test.WaitDuration, test.WaitTick)
}

func createDHT(t *testing.T, ctx context.Context, options ...lpdht.Option) (*lpdht.IpfsDHT, error) {
	t.Helper()
	host, err := libp2p.New(libp2p.ListenAddrStrings(testnetwork.RandomLocalPeerAddress))
	if err != nil {
		return nil, err
	}
	return newDHT(ctx, host, options...)
}
