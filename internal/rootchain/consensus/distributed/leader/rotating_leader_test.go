package leader

import (
	"context"
	"crypto/rand"
	mrand "math/rand"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_RootNodesNil(t *testing.T) {
	ls, err := NewRotatingLeader(nil, 1)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_EmptyRootNodes(t *testing.T) {
	p := peer.CreatePeer(t)
	p.Configuration().Validators = []p2ppeer.ID{}
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
	persistentPeers := make(p2ppeer.IDSlice, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		id, err := network.NodeIDFromPublicKeyBytes(pubKeyBytes)
		require.NoError(t, err)
		persistentPeers[i] = id
	}
	//sort by pubkeys
	sort.Sort(persistentPeers)
	conf := &network.PeerConfiguration{
		Address:    "/ip4/127.0.0.1/tcp/0",
		Validators: persistentPeers,
	}
	ctx := context.Background()
	peer, err := network.NewPeer(ctx, conf)
	require.NoError(t, err)
	ls, err := NewRotatingLeader(peer, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, 6, len(ls.GetRootNodes()))
	round := 0
	for round < 14 {
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round)), persistentPeers[round%nofPeers])
		round++
	}
}

func TestNewLeaderSelector_NormalTwoRounds(t *testing.T) {
	const nofPeers = 6
	persistentPeers := make(p2ppeer.IDSlice, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		id, err := network.NodeIDFromPublicKeyBytes(pubKeyBytes)
		require.NoError(t, err)
		persistentPeers[i] = id
	}
	//sort by pubkeys
	sort.Sort(persistentPeers)
	conf := &network.PeerConfiguration{
		Address:    "/ip4/127.0.0.1/tcp/0",
		Validators: persistentPeers,
	}
	ctx := context.Background()
	peer, err := network.NewPeer(ctx, conf)
	require.NoError(t, err)
	// two rounds have the same leader
	ls, err := NewRotatingLeader(peer, 2)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, 6, len(ls.GetRootNodes()))
	round := 0
	leaderIndex := 0
	for round < 14 {
		id := persistentPeers[leaderIndex%nofPeers]
		require.NoError(t, err)
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round)), id)
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round+1)), id)
		round += 2
		leaderIndex++
	}
}

func TestNewLeaderSelector_IsValidLeader(t *testing.T) {
	const nofPeers = 6
	persistentPeers := make(p2ppeer.IDSlice, nofPeers)
	for i := range persistentPeers {
		_, publicKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
		require.NoError(t, err)
		pubKeyBytes, err := publicKey.Raw()
		require.NoError(t, err)
		id, err := network.NodeIDFromPublicKeyBytes(pubKeyBytes)
		require.NoError(t, err)
		persistentPeers[i] = id
	}
	//sort by pubkeys
	sort.Sort(persistentPeers)
	conf := &network.PeerConfiguration{
		Address:    "/ip4/127.0.0.1/tcp/0",
		Validators: persistentPeers,
	}
	ctx := context.Background()
	peer, err := network.NewPeer(ctx, conf)
	require.NoError(t, err)
	ls, err := NewRotatingLeader(peer, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	test := 0
	for test < 10 {
		randomRound := mrand.Intn(1000)
		id := persistentPeers[randomRound%nofPeers]
		require.NoError(t, err)
		require.Equal(t, ls.GetLeaderForRound(uint64(randomRound)), id)
		test++
	}
}
