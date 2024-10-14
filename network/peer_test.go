package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

const randomTestAddressStr = "/ip4/127.0.0.1/tcp/0"

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
	// test stringer
	idStr := peer.ID().String()
	require.EqualValues(t, fmt.Sprintf("NodeID:%s*%s", idStr[:2], idStr[len(idStr)-6:]), peer.String())
	pubKeyFromID, err := peer.ID().ExtractPublicKey()
	require.NoError(t, err)
	require.Equal(t, pubKey, pubKeyFromID)
}

func TestBootstrapNodes(t *testing.T) {
	log := logger.New(t)
	ctx := context.Background()
	bootStrapPeerConf, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), nil, nil)
	require.NoError(t, err)

	bootstrapNode, err := NewPeer(ctx, bootStrapPeerConf, log, nil)
	require.NoError(t, err)
	bootstrapNodeAddrInfo := []peer.AddrInfo{{ID: bootstrapNode.ID(), Addrs: bootstrapNode.MultiAddresses()}}

	peerConf1, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer1, err := NewPeer(ctx, peerConf1, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer1.Close() }()
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 1 }, test.WaitDuration, test.WaitTick)

	peerConf2, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer2, err := NewPeer(ctx, peerConf2, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer2.Close() }()

	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Find(peer1.dht.Host().ID()) != "" }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Find(peer2.dht.Host().ID()) != "" }, test.WaitDuration, test.WaitTick)
}

func TestBootstrap_OneBootStrapConnectionFails_StillOK(t *testing.T) {
	log := logger.New(t)
	ctx := context.Background()
	bootStrapPeer1Conf, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), nil, nil)
	require.NoError(t, err)
	bootstrap1NodeAddr, err := ma.NewMultiaddr("/ip4/127.0.0.2/tcp/10")
	require.NoError(t, err)

	bootStrapPeer2Conf, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), nil, nil)
	require.NoError(t, err)
	bootstrapNode2, err := NewPeer(ctx, bootStrapPeer2Conf, log, nil)
	require.NoError(t, err)
	// set bootstrap info
	bootstrapNodeAddrInfo := []peer.AddrInfo{
		{ID: bootStrapPeer1Conf.ID, Addrs: []ma.Multiaddr{bootstrap1NodeAddr}},
		{ID: bootstrapNode2.ID(), Addrs: bootstrapNode2.MultiAddresses()},
	}

	peerConf1, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer1, err := NewPeer(ctx, peerConf1, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer1.Close() }()
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 1 }, 2*test.WaitDuration, test.WaitTick)

	peerConf2, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer2, err := NewPeer(ctx, peerConf2, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer2.Close() }()

	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Size() == 2 }, 2*test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 2 }, 2*test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Find(peer1.dht.Host().ID()) != "" }, 2*test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Find(peer2.dht.Host().ID()) != "" }, 2*test.WaitDuration, test.WaitTick)
}

func TestBootstrap_AllConnectionsFail(t *testing.T) {
	log := logger.New(t)
	ctx := context.Background()
	bootStrapPeer1Conf, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), nil, nil)
	require.NoError(t, err)
	addr, err := ma.NewMultiaddr("/ip4/127.0.0.2/tcp/10")
	require.NoError(t, err)

	bootstrapNodeAddrInfo := []peer.AddrInfo{{ID: bootStrapPeer1Conf.ID, Addrs: []ma.Multiaddr{addr}}}

	peerConf1, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)

	peer1, err := NewPeer(ctx, peerConf1, log, nil)
	require.NoError(t, err)
	require.NotNil(t, peer1)
	err = peer1.BootstrapConnect(ctx, log)
	require.ErrorContains(t, err, fmt.Sprintf("failed to bootstrap: failed to dial: failed to dial %s: all dials failed", bootStrapPeer1Conf.ID))
}

func TestProvidesAndDiscoverNodes(t *testing.T) {
	log := logger.New(t)
	ctx := context.Background()
	bootStrapPeerConf, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), nil, nil)
	require.NoError(t, err)
	bootstrapNode, err := NewPeer(ctx, bootStrapPeerConf, log, nil)
	require.NoError(t, err)
	bootstrapNodeAddrInfo := []peer.AddrInfo{{ID: bootstrapNode.ID(), Addrs: bootstrapNode.MultiAddresses()}}

	peerConf1, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)
	peer1, err := NewPeer(ctx, peerConf1, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer1.Close() }()
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 1 }, test.WaitDuration, test.WaitTick)

	peerConf2, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)
	peer2, err := NewPeer(ctx, peerConf2, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer2.Close() }()

	peerConf3, err := NewPeerConfiguration(randomTestAddressStr, nil, generateKeyPair(t), bootstrapNodeAddrInfo, nil)
	require.NoError(t, err)
	peer3, err := NewPeer(ctx, peerConf3, log, nil)
	require.NoError(t, err)
	defer func() { _ = peer3.Close() }()

	require.Eventually(t, func() bool { return peer2.dht.RoutingTable().Size() == 3 }, 2*test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return peer1.dht.RoutingTable().Size() == 3 }, 2*test.WaitDuration, test.WaitTick)
	testTopic := "ab/test/test_topic"
	require.NoError(t, peer2.Advertise(ctx, testTopic))
	require.NoError(t, peer1.Advertise(ctx, testTopic))

	// discover peers with the topic
	peerChan, err := peer3.Discover(ctx, testTopic)
	require.NoError(t, err)
	peers := make([]peer.AddrInfo, 0, 2)
	for p := range peerChan {
		peers = append(peers, p)
	}
	require.Contains(t, peers, peer.AddrInfo{ID: peer1.ID(), Addrs: peer1.MultiAddresses()})
	require.Contains(t, peers, peer.AddrInfo{ID: peer2.ID(), Addrs: peer2.MultiAddresses()})
	require.Len(t, peers, 2)
}

func TestAnnounceAddrs(t *testing.T) {
	ctx := context.Background()
	announceAddrs := []string{
		"/ip4/203.0.113.1/tcp/4001",
		"/ip4/203.0.113.1/tcp/4002",
	}
	conf, err := NewPeerConfiguration(randomTestAddressStr, announceAddrs, generateKeyPair(t), nil, nil)
	require.NoError(t, err)

	peer1, err := NewPeer(ctx, conf, logger.New(t), nil)
	require.NoError(t, err)

	actualAddrs := peer1.host.Addrs()
	require.Len(t, actualAddrs, 2)
	require.Equal(t, announceAddrs[0], actualAddrs[0].String())
	require.Equal(t, announceAddrs[1], actualAddrs[1].String())
}

/*
createPeer returns new Peer configured with random port on localhost and registers
cleanup for it (ie in the end of the test peer.Close is called).
*/
func createPeer(t *testing.T) *Peer {
	return createBootstrappedPeer(t, nil, []peer.ID{})
}

func createBootstrappedPeer(t *testing.T, bootstrapPeers []peer.AddrInfo, validators []peer.ID) *Peer {
	keyPair := generateKeyPair(t)
	peerID, err := NodeIDFromPublicKeyBytes(keyPair.PublicKey)
	require.NoError(t, err)

	validators = append(validators, peerID)
	peerConf, err := NewPeerConfiguration(randomTestAddressStr, nil, keyPair, bootstrapPeers, validators)
	require.NoError(t, err)

	p, err := NewPeer(context.Background(), peerConf, logger.New(t), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	return p
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
