package forwarder

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txbuffer"
	golog "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

const (
	TxBufferSize = 100
)

var (
	transfer = testtransaction.RandomBillTransfer()
	split    = testtransaction.RandomBillSplit()
)

type FixedLeader struct {
	Leader        *network.Peer
	Error         error
	LeaderUnknown bool
}

func init() {
	golog.SetAllLoggers(golog.LevelWarn) // change this to Debug if libp2p logs are needed
}

func (l *FixedLeader) NextLeader() (peer.ID, error) {
	if l.LeaderUnknown {
		return UnknownLeader, nil
	}
	if l.Error != nil {
		return "", l.Error
	}
	if l.Leader == nil {
		return "", errors.New("leader not configured")
	}
	return l.Leader.ID(), nil
}

func TestTxHandler_FollowerForwardsRequestToLeader(t *testing.T) {
	// init leader
	leader := InitPeer(t, nil)
	defer leader.Close()
	fixedLeader := &FixedLeader{Leader: leader}
	_, leaderTxBuffer := RegisterTxHandler(t, leader, fixedLeader)

	// init follower
	follower := InitPeer(t, CreateBootstrapConfiguration(t, leader))
	defer follower.Close()
	followerTxHandler, followerTxBuffer := RegisterTxHandler(t, follower, fixedLeader)
	defer followerTxHandler.Close()
	require.Eventually(t, func() bool { return follower.RoutingTableSize() == 1 }, test.WaitDuration, test.WaitTick)

	// send requests
	err := followerTxHandler.Handle(context.Background(), transfer)
	require.NoError(t, err)
	err = followerTxHandler.Handle(context.Background(), split)
	require.NoError(t, err)

	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 2 }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { return followerTxBuffer.Count() == 0 }, test.WaitDuration, test.WaitTick)
}

func TestTxHandler_NextLeaderSelectorReturnsError(t *testing.T) {
	// init follower
	noLeaderErr := errors.New("no leader")
	fixedLeader := &FixedLeader{Error: noLeaderErr}
	follower := InitPeer(t, nil)
	defer follower.Close()
	followerTxHandler, _ := RegisterTxHandler(t, follower, fixedLeader)

	// send requests
	err := followerTxHandler.Handle(context.Background(), transfer)
	require.Error(t, err)
	require.EqualError(t, err, noLeaderErr.Error())
}

func TestTxHandler_NextLeaderIsUnknown(t *testing.T) {
	// init follower
	fixedLeader := &FixedLeader{LeaderUnknown: true}
	follower := InitPeer(t, nil)
	defer follower.Close()
	followerTxHandler, followerTxPool := RegisterTxHandler(t, follower, fixedLeader)

	// send requests
	err := followerTxHandler.Handle(context.Background(), transfer)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return followerTxPool.Count() == 1 }, test.WaitDuration, test.WaitTick)
}

func TestTxHandler_LeaderChangedWhenTxWasForwarded(t *testing.T) {
	leader2 := InitPeer(t, nil)

	// init leader
	leader := InitPeer(t, nil)
	defer leader.Close()
	errLeader := &FixedLeader{Leader: leader2}
	_, leaderTxBuffer := RegisterTxHandler(t, leader, errLeader)

	// init follower
	follower := InitPeer(t, CreateBootstrapConfiguration(t, leader))
	defer follower.Close()
	fixedLeader := &FixedLeader{leader, nil, false}
	followerTxHandler, _ := RegisterTxHandler(t, follower, fixedLeader)
	require.Eventually(t, func() bool { return follower.RoutingTableSize() == 1 }, test.WaitDuration, test.WaitTick)

	// send requests
	err := followerTxHandler.Handle(context.Background(), transfer)
	require.NoError(t, err)

	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 0 }, test.WaitDuration, test.WaitTick)
}

func TestTxHandler_LeaderFailsLeaderCheck(t *testing.T) {
	// init leader
	leader := InitPeer(t, nil)
	defer leader.Close()
	noLeaderErr := errors.New("no leader")
	errLeader := &FixedLeader{Error: noLeaderErr}
	_, leaderTxBuffer := RegisterTxHandler(t, leader, errLeader)

	// init follower
	follower := InitPeer(t, CreateBootstrapConfiguration(t, leader))
	defer follower.Close()
	fixedLeader := &FixedLeader{Leader: leader}
	followerTxHandler, _ := RegisterTxHandler(t, follower, fixedLeader)
	require.Eventually(t, func() bool { return follower.RoutingTableSize() == 1 }, test.WaitDuration, test.WaitTick)

	// send requests
	err := followerTxHandler.Handle(context.Background(), transfer)
	require.NoError(t, err)

	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 0 }, test.WaitDuration, test.WaitTick)
}

func TestTxHandler_LeaderDiesAndComesBack(t *testing.T) {
	privKey, pubKey, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKeyBytes, _ := crypto.MarshalPrivateKey(privKey)
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)

	leaderConf := &network.PeerConfiguration{
		KeyPair: &network.PeerKeyPair{
			Priv: privKeyBytes,
			Pub:  pubKeyBytes,
		},
	}

	// init leader
	leader := InitPeer(t, leaderConf)
	fixedLeader := &FixedLeader{Leader: leader}
	_, leaderTxBuffer := RegisterTxHandler(t, leader, fixedLeader)

	// init follower
	follower := InitPeer(t, CreateBootstrapConfiguration(t, leader))
	defer follower.Close()
	txHandler, _ := RegisterTxHandler(t, follower, fixedLeader)
	require.Eventually(t, func() bool { return follower.RoutingTableSize() == 1 }, test.WaitDuration, test.WaitTick)
	// send requests
	err := txHandler.Handle(context.Background(), transfer)
	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 1 }, test.WaitDuration, test.WaitTick)

	leaderAddress := leader.MultiAddresses()[0].String()
	// shut down leader

	err = leader.Close()
	require.NoError(t, err)
	leaderConf.Address = leaderAddress

	// init leader again
	leader = InitPeer(t, leaderConf)
	defer leader.Close()
	_, err = New(leader, fixedLeader, leaderTxBuffer)
	require.NoError(t, err)
	// send transactions to follower
	err = txHandler.Handle(context.Background(), testtransaction.RandomBillTransfer())
	require.NoError(t, err)
	err = txHandler.Handle(context.Background(), testtransaction.RandomBillTransfer())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 3 }, test.WaitDuration, test.WaitTick)

}

func TestTxHandler_LeaderDoesNotForward(t *testing.T) {
	// init leader
	leader := InitPeer(t, nil)
	defer leader.Close()
	fixedLeader := &FixedLeader{Leader: leader}
	txHandler, leaderTxBuffer := RegisterTxHandler(t, leader, fixedLeader)

	// send requests
	err := txHandler.Handle(context.Background(), transfer)
	require.NoError(t, err)
	err = txHandler.Handle(context.Background(), split)
	require.NoError(t, err)
	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 2 }, test.WaitDuration, test.WaitTick)
}

func TestTxHandler_LeaderDown(t *testing.T) {
	// init leader
	leader := InitPeer(t, nil)
	fixedLeader := &FixedLeader{Leader: leader}
	_, leaderTxBuffer := RegisterTxHandler(t, leader, fixedLeader)

	// init follower
	follower := InitPeer(t, CreateBootstrapConfiguration(t, leader))
	defer follower.Close()
	txHandler, _ := RegisterTxHandler(t, follower, fixedLeader)
	require.Eventually(t, func() bool { return follower.RoutingTableSize() == 1 }, test.WaitDuration, test.WaitTick)
	// shut down leader
	err := leader.Close()
	require.NoError(t, err)
	// send requests
	err = txHandler.Handle(context.Background(), transfer)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "failed to dial"))

	// verify tx buffers
	require.Eventually(t, func() bool { return leaderTxBuffer.Count() == 0 }, test.WaitDuration, test.WaitTick)
}

func RegisterTxHandler(t *testing.T, p *network.Peer, leaderSelector LeaderSelector) (*TxForwarder, *txbuffer.TxBuffer) {
	t.Helper()
	b, err := txbuffer.New(TxBufferSize)
	require.NoError(t, err)
	handler, err := New(p, leaderSelector, b)
	require.NoError(t, err)
	return handler, b
}

func CreateBootstrapConfiguration(t *testing.T, p *network.Peer) *network.PeerConfiguration {
	t.Helper()
	return &network.PeerConfiguration{BootstrapPeers: []*network.PeerInfo{{
		ID:      peer.Encode(p.ID()),
		Address: p.MultiAddresses()[0].String(),
	}}}
}

func InitPeer(t *testing.T, conf *network.PeerConfiguration) *network.Peer {
	t.Helper()
	ctx := context.Background()
	peer, err := network.NewPeer(ctx, conf)
	require.NoError(t, err)
	return peer
}
