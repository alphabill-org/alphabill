package peer

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/alphabill-org/alphabill/validator/pkg/network"
	"github.com/alphabill-org/alphabill/validator/pkg/testutils/logger"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
)

func CreatePeerConfiguration(t *testing.T) *network.PeerConfiguration {
	peerConf, err := network.NewPeerConfiguration("/ip4/127.0.0.1/tcp/0", generateKeyPair(t), nil, nil)
	require.NoError(t, err)
	return peerConf
}

func CreatePeer(t *testing.T, peerConf *network.PeerConfiguration) *network.Peer {
	peer, err := network.NewPeer(context.Background(), peerConf, logger.New(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, peer.Close()) })
	return peer
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
