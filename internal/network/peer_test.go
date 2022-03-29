package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/require"
)

func TestNewPeer_PeerConfigurationIsNil(t *testing.T) {
	ctx := context.Background()
	peer, err := NewPeer(ctx, nil)
	require.ErrorIs(t, err, ErrPeerConfigurationIsNil)
	require.Nil(t, peer)
}

func TestNewPeer_GeneratesKeys(t *testing.T) {
	ctx := context.Background()
	peer, err := NewPeer(ctx, &PeerConfiguration{})
	require.NoError(t, err)
	defer peer.Close()
	require.NotNil(t, peer)
	require.NotNil(t, peer.ID())
	require.True(t, len(peer.MultiAddresses()) > 0)
	require.Equal(t, 1, len(peer.host.Peerstore().Peers()))
}

func TestNewPeer_WithPersistentPeers(t *testing.T) {
	ctx := context.Background()
	peers, err := createPeers(4)
	defer func() {
		for _, peer := range peers {
			if peer != nil {
				peer.Close()
			}
		}
	}()
	require.NoError(t, err)
	pis := make([]*PeerInfo, len(peers))
	for i, peer := range peers {
		pubKey, err := peer.PublicKey()
		require.NoError(t, err)

		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		require.NoError(t, err)

		pis[i] = &PeerInfo{
			Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
			PublicKey: pubKeyBytes,
		}
	}

	peer, err := NewPeer(ctx, &PeerConfiguration{
		Address:         "",
		KeyPair:         nil,
		PersistentPeers: pis,
	})
	require.NoError(t, err)
	defer peer.Close()
	require.NotNil(t, peer)
	require.NotNil(t, peer.ID())
	require.True(t, len(peer.MultiAddresses()) > 0)
	require.Equal(t, 5, len(peer.host.Peerstore().Peers()))
}

func TestNewPeer_InvalidPrivateKey(t *testing.T) {
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: test.RandomBytes(30),
		},
	}
	_, err := NewPeer(ctx, conf)
	require.Error(t, err)
	require.Errorf(t, err, ErrStringInvalidPrivateKey)
}

func TestNewPeer_InvalidPublicKey(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKeyBytes, _ := crypto.MarshalPrivateKey(privKey)
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: privKeyBytes,
			PublicKey:  test.RandomBytes(30),
		},
	}
	_, err := NewPeer(ctx, conf)
	require.Error(t, err)
	require.Errorf(t, err, ErrStringInvalidPublicKey)
}

func TestNewPeer_LoadsKeyPairCorrectly(t *testing.T) {
	privateKey, pubKey, _ := crypto.GenerateEd25519Key(rand.Reader)
	keyBytes, err := crypto.MarshalPrivateKey(privateKey)
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
	ctx := context.Background()
	conf := &PeerConfiguration{
		KeyPair: &PeerKeyPair{
			PrivateKey: keyBytes,
			PublicKey:  pubKeyBytes,
		},
	}
	peer, err := NewPeer(ctx, conf)
	require.NoError(t, err)
	p := peer.host.Peerstore().PeersWithKeys()[0]
	pub, _ := p.ExtractPublicKey()
	raw, _ := pub.Raw()
	require.Equal(t, pubKeyBytes[4:], raw)
}

func createPeers(nrOfPeers int) ([]*Peer, error) {
	peers := make([]*Peer, nrOfPeers)
	for i := 0; i < nrOfPeers; i++ {
		p, err := createPeer()
		if err != nil {
			return peers, err
		}
		peers[i] = p
	}
	return peers, nil
}

func createPeer() (*Peer, error) {
	ctx := context.Background()
	return NewPeer(ctx, &PeerConfiguration{})
}
