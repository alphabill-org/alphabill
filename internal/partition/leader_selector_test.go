package partition

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_PeerIsNil(t *testing.T) {
	ls, err := NewLeaderSelector(nil)
	require.ErrorIs(t, err, ErrPeerIsNilIndex)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_Ok(t *testing.T) {
	ls, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, UnknownLeader, ls.leader.String())
	require.Equal(t, UnknownLeader, ls.GetLeader().String())
}

func TestLeaderSelector_SelfID(t *testing.T) {
	peer := createPeer(t)
	ls, err := NewLeaderSelector(peer)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, peer.ID(), ls.SelfID())
}

func TestLeaderSelector_IsCurrentNodeLeader(t *testing.T) {
	peer := createPeer(t)
	ls, err := NewLeaderSelector(peer)
	require.NoError(t, err)
	require.NotNil(t, ls)
}
