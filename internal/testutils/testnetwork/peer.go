package testnetwork

import (
	"fmt"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/require"
)

func CreatePeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}
