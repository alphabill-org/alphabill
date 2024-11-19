package peer

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	p2ptest "github.com/libp2p/go-libp2p/core/test"
	"github.com/stretchr/testify/require"
)

func CreatePeerConfiguration(t *testing.T) *network.PeerConfiguration {
	peerConf, err := network.NewPeerConfiguration("/ip4/127.0.0.1/tcp/0", nil, generateKeyPair(t), nil)
	require.NoError(t, err)
	return peerConf
}

func CreatePeer(t *testing.T, peerConf *network.PeerConfiguration) *network.Peer {
	peer, err := network.NewPeer(context.Background(), peerConf, logger.New(t), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, peer.Close()) })
	return peer
}

func GeneratePeerIDs(t *testing.T, count int) peer.IDSlice {
	t.Helper()
	var peers = make(peer.IDSlice, count)
	for i := 0; i < count; i++ {
		id, err := p2ptest.RandPeerID()
		require.NoError(t, err)
		peers[i] = id
	}
	return peers
}

func generateKeyPair(t *testing.T) *network.PeerKeyPair {
	privateKey, publicKey, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
	privateKeyBytes, err := privateKey.Raw()
	require.NoError(t, err)
	publicKeyBytes, err := publicKey.Raw()
	require.NoError(t, err)

	return &network.PeerKeyPair{
		PublicKey:  publicKeyBytes,
		PrivateKey: privateKeyBytes,
	}
}
