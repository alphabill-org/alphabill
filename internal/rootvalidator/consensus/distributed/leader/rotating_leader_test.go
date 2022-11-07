package leader

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_RootNodesNil(t *testing.T) {
	ls, err := NewRotatingLeader(nil, 1)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_EmptyRootNodes(t *testing.T) {
	var rootNodes []peer.ID
	ls, err := NewRotatingLeader(rootNodes, 1)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_NofRoundsZero(t *testing.T) {
	var rootNodes []peer.ID
	ls, err := NewRotatingLeader(rootNodes, 0)
	require.Error(t, err)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_Normal(t *testing.T) {
	rootNodes := make([]peer.ID, 6)
	for i := range rootNodes {
		rootNodes[i] = peer.ID(strconv.Itoa(i))
	}
	ls, err := NewRotatingLeader(rootNodes, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.EqualValues(t, ls.GetLeaderForRound(0), "0")
	require.EqualValues(t, ls.GetLeaderForRound(1), "1")
	require.EqualValues(t, ls.GetLeaderForRound(5), "5")
	require.EqualValues(t, ls.GetLeaderForRound(6), "0")
	require.EqualValues(t, ls.GetLeaderForRound(7), "1")
}

func TestNewLeaderSelector_NormalTwoRounds(t *testing.T) {
	rootNodes := make([]peer.ID, 6)
	for i := range rootNodes {
		rootNodes[i] = peer.ID(strconv.Itoa(i))
	}
	ls, err := NewRotatingLeader(rootNodes, 2)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.EqualValues(t, ls.GetLeaderForRound(0), "0")
	require.EqualValues(t, ls.GetLeaderForRound(1), "0")
	require.EqualValues(t, ls.GetLeaderForRound(4), "2")
	require.EqualValues(t, ls.GetLeaderForRound(5), "2")
	require.EqualValues(t, ls.GetLeaderForRound(6), "3")
	require.EqualValues(t, ls.GetLeaderForRound(7), "3")
	require.EqualValues(t, ls.GetLeaderForRound(10), "5")
	require.EqualValues(t, ls.GetLeaderForRound(11), "5")
	require.EqualValues(t, ls.GetLeaderForRound(12), "0")
	require.EqualValues(t, ls.GetLeaderForRound(13), "0")
}

func TestNewLeaderSelector_IsValidLeader(t *testing.T) {
	rootNodes := make([]peer.ID, 6)
	for i, _ := range rootNodes {
		rootNodes[i] = peer.ID(strconv.Itoa(i))
	}
	ls, err := NewRotatingLeader(rootNodes, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.True(t, ls.IsValidLeader("0", 0))
	require.True(t, ls.IsValidLeader("0", 30))
	require.True(t, ls.IsValidLeader("5", 41))
	require.False(t, ls.IsValidLeader("0", 40))
}
