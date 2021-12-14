package network

import (
	"context"
	"crypto/rand"
	"fmt"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNewPeer_GeneratesKeys(t *testing.T) {
	ctx := context.Background()
	peer, err := NewPeer(ctx, nil)
	require.NoError(t, err)
	defer peer.Close()
	require.NotNil(t, peer)
	require.NotNil(t, peer.ID())
	require.True(t, len(peer.MultiAddresses()) > 0)
	require.True(t, peer.RoutingTableSize() == 0)
	require.Equal(t, 1, len(peer.host.Peerstore().Peers()))
}

func TestNewPeer_InvalidPrivateKey(t *testing.T) {
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			Priv: test.RandomBytes(30),
		},
	}
	_, err := NewPeer(ctx, conf)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), ErrStringInvalidPrivateKey))
}

func TestNewPeer_InvalidPublicKey(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKeyBytes, _ := crypto.MarshalPrivateKey(privKey)
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			Priv: privKeyBytes,
			Pub:  test.RandomBytes(30),
		},
	}
	_, err := NewPeer(ctx, conf)
	require.Error(t, err)
	fmt.Println(err)
	require.True(t, strings.Contains(err.Error(), ErrStringInvalidPublicKey))
}

func TestNewPeer_LoadsKeyPairCorrectly(t *testing.T) {
	privateKey, pubKey, _ := crypto.GenerateEd25519Key(rand.Reader)
	keyBytes, err := crypto.MarshalPrivateKey(privateKey)
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			Priv: keyBytes,
			Pub:  pubKeyBytes,
		},
	}
	peer, err := NewPeer(ctx, conf)
	require.NoError(t, err)
	p := peer.host.Peerstore().PeersWithKeys()[0]
	pub, _ := p.ExtractPublicKey()
	raw, _ := pub.Raw()
	require.Equal(t, pubKeyBytes[4:], raw)
}

