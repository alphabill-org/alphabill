package partition

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/peer"
	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_PeerIsNil(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(nil, systemIdentifier)
	require.ErrorIs(t, err, ErrPeerIsNil)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_SystemIdentifierIsNil(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(peer.CreatePeer(t), nil)
	require.ErrorContains(t, err, ErrStrSystemIdentifierIsNil)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_Ok(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(peer.CreatePeer(t), systemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, UnknownLeader, ls.leader.String())
}

func TestLeaderSelector_SelfID(t *testing.T) {
	p := peer.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(p, systemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, p.ID(), ls.SelfID())
}

func TestLeaderSelector_IsCurrentNodeLeader(t *testing.T) {
	p := peer.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(p, systemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, ls)
}
