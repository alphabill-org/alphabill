package partition

import (
	"testing"

	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"

	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_PeerIsNil(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(nil)
	require.ErrorIs(t, err, ErrPeerIsNil)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_Ok(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(testnetwork.CreatePeer(t))
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, UnknownLeader, ls.leader.String())
}

func TestLeaderSelector_SelfID(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(p)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, p.ID(), ls.SelfID())
}

func TestLeaderSelector_IsCurrentNodeLeader(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(p)
	require.NoError(t, err)
	require.NotNil(t, ls)
}
