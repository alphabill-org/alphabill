package leader

import (
	"math/rand"
	"sort"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/stretchr/testify/require"

	p2ppeer "github.com/libp2p/go-libp2p/core/peer"
)

func TestNewRoundRobin_RootNodesNil(t *testing.T) {
	ls, err := NewRoundRobin(nil, 1)
	require.EqualError(t, err, `empty root validator node id list`)
	require.Nil(t, ls)
}

func TestNewRoundRobin_EmptyRootNodes(t *testing.T) {
	ls, err := NewRoundRobin([]p2ppeer.ID{}, 1)
	require.EqualError(t, err, `empty root validator node id list`)
	require.Nil(t, ls)
}

func TestNewRoundRobin_NofRoundsZero(t *testing.T) {
	ls, err := NewRoundRobin([]p2ppeer.ID{"A"}, 0)
	require.EqualError(t, err, `invalid number of continuous rounds 0 (must be between 1 and 1)`)
	require.Nil(t, ls)

	ls, err = NewRoundRobin([]p2ppeer.ID{"A", "B"}, 3)
	require.EqualError(t, err, `invalid number of continuous rounds 3 (must be between 1 and 2)`)
	require.Nil(t, ls)
}

func TestRoundRobin_Normal(t *testing.T) {
	const nofPeers = 6
	persistentPeers := test.GeneratePeerIDs(t, nofPeers)
	sort.Sort(persistentPeers)

	ls, err := NewRoundRobin(persistentPeers, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)

	round := 0
	for round < 14 {
		require.EqualValues(t, ls.GetLeaderForRound(uint64(round)), persistentPeers[round%nofPeers])
		round++
	}
}

func TestRoundRobin_NormalTwoRounds(t *testing.T) {
	const nofPeers = 6
	persistentPeers := test.GeneratePeerIDs(t, nofPeers)
	sort.Sort(persistentPeers)

	// two rounds have the same leader
	ls, err := NewRoundRobin(persistentPeers, 2)
	require.NoError(t, err)
	require.NotNil(t, ls)

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

func TestRoundRobin_IsValidLeader(t *testing.T) {
	const nofPeers = 6
	persistentPeers := test.GeneratePeerIDs(t, nofPeers)
	sort.Sort(persistentPeers)

	ls, err := NewRoundRobin(persistentPeers, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	test := 0
	for test < 10 {
		randomRound := rand.Intn(1000)
		id := persistentPeers[randomRound%nofPeers]
		require.NoError(t, err)
		require.Equal(t, ls.GetLeaderForRound(uint64(randomRound)), id)
		test++
	}
}

func TestRoundRobin_Update(t *testing.T) {
	ls, err := NewRoundRobin([]p2ppeer.ID{"A", "B"}, 1)
	require.NoError(t, err)
	require.NotNil(t, ls)
	// Update is NOP in round-robin so should just succeed
	require.NoError(t, ls.Update(nil, 0, nil))
	require.NoError(t, ls.Update(nil, 100, nil))
}
