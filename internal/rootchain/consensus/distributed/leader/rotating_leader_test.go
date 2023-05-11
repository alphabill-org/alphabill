package leader

import (
	"crypto/rand"
	mrand "math/rand"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_RootNodesNil(t *testing.T) {
	ls, err := NewRotatingLeader(nil, 1)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_EmptyRootNodes(t *testing.T) {
	p := peer.CreatePeer(t)
	p.Configuration().PersistentPeers = []*network.PeerInfo{}
	ls, err := NewRotatingLeader(p, 1)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_NofRoundsZero(t *testing.T) {
	p := peer.CreatePeer(t)
	ls, err := NewRotatingLeader(p, 0)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_ContZero(t *testing.T) {
	p := peer.CreatePeer(t)
	_, err := NewRotatingLeader(p, 0)
	require.Error(t, err)
}

func TestNewLeaderSelector_Normal(t *testing.T) {
	const nofPeers = 6
	persistentPeers := make([]*network.PeerInfo, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		persistentPeers[i] = &network.PeerInfo{
			Address:   "/ip4/127.0.0.1/tcp/0", // address of the peer
			PublicKey: pubKeyBytes,            // peer public key [0],[1]...
		}
	}
	//sort by pubkeys
	sort.Slice(persistentPeers, func(i, j int) bool {
		return string(persistentPeers[i].PublicKey) < string(persistentPeers[j].PublicKey)
	})
	conf := &network.PeerConfiguration{
		Address:         "/ip4/127.0.0.1/tcp/0",
		PersistentPeers: persistentPeers,
	}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)
	ls, err := NewRotatingLeader(peer, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, 6, len(ls.GetRootNodes()))
	round := 0
	for round < 14 {
		id, err := persistentPeers[round%nofPeers].GetID()
		require.NoError(t, err)
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round)), id)
		round++
	}
}

func TestNewLeaderSelector_NormalTwoRounds(t *testing.T) {
	const nofPeers = 6
	persistentPeers := make([]*network.PeerInfo, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		persistentPeers[i] = &network.PeerInfo{
			Address:   "/ip4/127.0.0.1/tcp/0", // address of the peer
			PublicKey: pubKeyBytes,            // peer public key [0],[1]...
		}
	}
	//sort by pubkeys
	sort.Slice(persistentPeers, func(i, j int) bool {
		return string(persistentPeers[i].PublicKey) < string(persistentPeers[j].PublicKey)
	})
	conf := &network.PeerConfiguration{
		Address:         "/ip4/127.0.0.1/tcp/0",
		PersistentPeers: persistentPeers,
	}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)
	// two rounds have the same leader
	ls, err := NewRotatingLeader(peer, 2)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, 6, len(ls.GetRootNodes()))
	round := 0
	leaderIndex := 0
	for round < 14 {
		id, err := persistentPeers[leaderIndex%nofPeers].GetID()
		require.NoError(t, err)
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round)), id)
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round+1)), id)
		round += 2
		leaderIndex++
	}
}

func TestNewLeaderSelector_IsValidLeader(t *testing.T) {
	const nofPeers = 6
	persistentPeers := make([]*network.PeerInfo, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		persistentPeers[i] = &network.PeerInfo{
			Address:   "/ip4/127.0.0.1/tcp/0", // address of the peer
			PublicKey: pubKeyBytes,            // peer public key [0],[1]...
		}
	}
	//sort by pubkeys
	sort.Slice(persistentPeers, func(i, j int) bool {
		return string(persistentPeers[i].PublicKey) < string(persistentPeers[j].PublicKey)
	})
	conf := &network.PeerConfiguration{
		Address:         "/ip4/127.0.0.1/tcp/0",
		PersistentPeers: persistentPeers,
	}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)
	ls, err := NewRotatingLeader(peer, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	test := 0
	for test < 10 {
		randomRound := mrand.Intn(1000)
		id, err := persistentPeers[randomRound%nofPeers].GetID()
		require.NoError(t, err)
		require.Equal(t, ls.GetLeaderForRound(uint64(randomRound)), id)
		test++
	}
}
