package network

import (
	"context"
	"crypto/rand"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
)

const randomTestAddressStr = "/ip4/127.0.0.1/tcp/0"

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

func TestNewPeer_PeerConfigurationIsNil(t *testing.T) {
	p, err := NewPeer(context.Background(), nil, nil, nil)
	require.ErrorIs(t, err, ErrPeerConfigurationIsNil)
	require.Nil(t, p)
}

func TestNewPeer_NewPeerCanBeCreated(t *testing.T) {
	p := createPeer(t)
	require.NotNil(t, p)
	require.NotNil(t, p.ID())
	require.True(t, len(p.MultiAddresses()) > 0)
	require.Equal(t, 1, len(p.host.Peerstore().Peers()))
}

func TestNewPeer_InvalidPrivateKey(t *testing.T) {
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: test.RandomBytes(30),
		},
	}

	_, err := NewPeer(context.Background(), conf, logger.New(t), nil)
	require.ErrorContains(t, err, "invalid private key: expected secp256k1 data size to be 32")
}

func TestNewPeer_InvalidPublicKey(t *testing.T) {
	privKey, _, _ := crypto.GenerateSecp256k1Key(rand.Reader)
	privKeyBytes, _ := privKey.Raw()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: privKeyBytes,
			PublicKey:  test.RandomBytes(30),
		},
	}
	_, err := NewPeer(context.Background(), conf, logger.New(t), nil)
	require.ErrorContains(t, err, "invalid public key: malformed public key: invalid length: 30")
}

func TestNewPeer_LoadsKeyPairCorrectly(t *testing.T) {
	privateKey, pubKey, _ := crypto.GenerateSecp256k1Key(rand.Reader)
	keyBytes, err := privateKey.Raw()
	require.NoError(t, err)
	pubKeyBytes, err := pubKey.Raw()
	require.NoError(t, err)
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: keyBytes,
			PublicKey:  pubKeyBytes,
		},
		Address: randomTestAddressStr,
	}
	peer, err := NewPeer(context.Background(), conf, logger.New(t), nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, peer.Close()) }()
	p := peer.host.Peerstore().PeersWithKeys()[0]
	pub, _ := p.ExtractPublicKey()
	raw, _ := pub.Raw()
	require.Equal(t, pubKeyBytes, raw)
}

func TestBootstrapNodes(t *testing.T) {
	log := logger.New(t)
	ctx := context.Background()
	bootstrapNode := createDHT(ctx, t, false, dht.DisableAutoRefresh())
	bootstrapNodeAddrInfo := []peer.AddrInfo{{ID: bootstrapNode.Host().ID(), Addrs: bootstrapNode.Host().Addrs()}}

	peerConf1, err := NewPeerConfiguration(randomTestAddressStr, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer1, err := NewPeer(context.Background(), peerConf1, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer1.Close() }()
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 1 }, test.WaitDuration, test.WaitTick)

	peerConf2, err := NewPeerConfiguration(randomTestAddressStr, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer2, err := NewPeer(context.Background(), peerConf2, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer2.Close() }()

	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Find(peer1.dht.Host().ID()) != "" }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Find(peer2.dht.Host().ID()) != "" }, test.WaitDuration, test.WaitTick)
}

/*
createPeer returns new Peer configured with random port on localhost and registers
cleanup for it (ie in the end of the test peer.Close is called).
*/
func createPeer(t *testing.T) *Peer {
	peerConf, err := NewPeerConfiguration(randomTestAddressStr, generateKeyPair(t), nil, nil)
	require.NoError(t, err)

	p, err := NewPeer(context.Background(), peerConf, logger.New(t), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	return p
}

func createDHT(ctx context.Context, t *testing.T, client bool, options ...dht.Option) *dht.IpfsDHT {
	baseOpts := []dht.Option{
		dht.DisableAutoRefresh(),
		dht.Validator(&blankValidator{}),
		dht.ProtocolPrefix(dhtProtocolPrefix),
	}

	if client {
		baseOpts = append(baseOpts, dht.Mode(dht.ModeClient))
	} else {
		baseOpts = append(baseOpts, dht.Mode(dht.ModeServer))
	}

	host := createHost(t)

	d, err := dht.New(ctx, host, append(baseOpts, options...)...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	return d
}

func createHost(t *testing.T) *basichost.BasicHost {
	host, err := basichost.NewHost(swarmt.GenSwarm(t, swarmt.OptDisableReuseport), new(basichost.HostOpts))
	require.NoError(t, err)
	host.Start()
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	return host
}

func generateKeyPair(t *testing.T) *PeerKeyPair {
	privateKey, publicKey, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
	privateKeyBytes, err := privateKey.Raw()
	require.NoError(t, err)
	publicKeyBytes, err := publicKey.Raw()
	require.NoError(t, err)

	return &PeerKeyPair{
		PublicKey:  publicKeyBytes,
		PrivateKey: privateKeyBytes,
	}
}
