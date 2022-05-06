package partition

import (
	"testing"

	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"

	"github.com/libp2p/go-libp2p-core/peer"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_PeerIsNil(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(nil, eventbus.New())
	require.ErrorIs(t, err, ErrPeerIsNil)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_EventBusIsNil(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(testnetwork.CreatePeer(t), nil)
	require.ErrorIs(t, err, ErrEventBusIsNil)
	require.Nil(t, ls)
}

func TestNewLeaderSelector_Ok(t *testing.T) {
	ls, err := NewDefaultLeaderSelector(testnetwork.CreatePeer(t), eventbus.New())
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, UnknownLeader, ls.leader.String())
}

func TestLeaderSelector_SelfID(t *testing.T) {
	peer := testnetwork.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(peer, eventbus.New())
	require.NoError(t, err)
	require.NotNil(t, ls)
	require.Equal(t, peer.ID(), ls.SelfID())
}

func TestLeaderSelector_IsCurrentNodeLeader(t *testing.T) {
	peer := testnetwork.CreatePeer(t)
	ls, err := NewDefaultLeaderSelector(peer, eventbus.New())
	require.NoError(t, err)
	require.NotNil(t, ls)
}

func TestLeaderSelector_SendsEventsToLeaderChangeTopic(t *testing.T) {
	bus := eventbus.New()
	leaderCh, err := bus.Subscribe(eventbus.TopicLeaders, 10)
	require.NoError(t, err)
	ls, err := NewDefaultLeaderSelector(testnetwork.CreatePeer(t), bus)
	require.NoError(t, err)
	leaderID := peer.ID("new_leader")
	ls.setLeader(leaderID)
	require.Eventually(t, func() bool {
		leader := <-leaderCh
		return leaderID == leader.(eventbus.NewLeaderEvent).NewLeader
	}, test.WaitDuration, test.WaitTick)

	ls.setLeader(UnknownLeader)
	require.Eventually(t, func() bool {
		leader := <-leaderCh
		return UnknownLeader == leader.(eventbus.NewLeaderEvent).NewLeader
	}, test.WaitDuration, test.WaitTick)

}
