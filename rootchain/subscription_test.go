package rootchain

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

func Test_Subscriptions(t *testing.T) {
	certResp := &certification.CertificationResponse{
		Partition: 1,
		Shard:     types.ShardID{},
		Technical: certification.TechnicalRecord{Round: 666},
		UC: types.UnicityCertificate{
			InputRecord: &types.InputRecord{
				Hash:      []byte{1, 1, 1, 1, 1},
				BlockHash: []byte{2, 2, 2, 2, 2},
			},
		},
	}
	crKey := partitionShard{certResp.Partition, certResp.Shard.Key()}

	nodeIdA := generateNodeID(t)
	nodeIdB := generateNodeID(t)

	t.Run("subscribe", func(t *testing.T) {
		sender := func(ctx context.Context, msg any, receivers ...peer.ID) error {
			t.Error("unexpected send call", msg, receivers)
			return nil
		}
		subs, err := NewSubscriptions(sender, observability.Default(t))
		require.NoError(t, err)
		require.NotNil(t, subs)

		// attempt to subscribe invalid node ID
		require.EqualError(t, subs.Subscribe(1, types.ShardID{}, ""), `invalid receiver id: failed to parse peer ID: invalid cid: cid too short`)
		require.EqualError(t, subs.Subscribe(1, types.ShardID{}, "foobar"), `invalid receiver id: failed to parse peer ID: invalid cid: encoding/hex: invalid byte: U+006F 'o'`)
		require.Len(t, subs.subs, 0, "partition/shard count")

		// valid ID 1
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdA.String()))
		require.Len(t, subs.subs, 1, "partition/shard count")
		require.Len(t, subs.subs[crKey], 1, "shard nodes")
		require.EqualValues(t, responsesPerSubscription, subs.subs[crKey][nodeIdA])

		// same ID 1 again, values should stay the same
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdA.String()))
		require.Len(t, subs.subs, 1, "partition/shard count")
		require.Len(t, subs.subs[crKey], 1, "shard nodes")
		require.EqualValues(t, responsesPerSubscription, subs.subs[crKey][nodeIdA])

		// add valid ID 2 of the same partition/shard
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdB.String()))
		require.Len(t, subs.subs, 1, "partition/shard count")
		require.Len(t, subs.subs[crKey], 2, "shard nodes")
		require.EqualValues(t, responsesPerSubscription, subs.subs[crKey][nodeIdA])
		require.EqualValues(t, responsesPerSubscription, subs.subs[crKey][nodeIdB])
	})

	t.Run("send, not subscribed", func(t *testing.T) {
		sender := func(ctx context.Context, msg any, receivers ...peer.ID) error {
			t.Error("unexpected send call", msg, receivers)
			return nil
		}

		// when there is no subscribers the "send" callback is not triggered but
		// debug message is always logged - so hook to logger to detect that
		msgCh := make(chan struct{})
		logHook := func(ctx context.Context, r slog.Record) {
			if r.Level == slog.LevelDebug && strings.HasPrefix(r.Message, `sending CertificationResponse, 0 receivers`) {
				msgCh <- struct{}{}
			}
		}
		obs := observability.New(t, "", "", logger.HookedLoggerBuilder(t, logHook))
		subs, err := NewSubscriptions(sender, obs)
		require.NoError(t, err)
		require.NotNil(t, subs)

		// no subscribers so should not trigger sender callback
		subs.Send(t.Context(), certResp)
		select {
		case <-msgCh:
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("haven't got log message, timeout")
		}

		// subscribe to some other partition, shouldn't trigger callback either
		require.NoError(t, subs.Subscribe(certResp.Partition+1, certResp.Shard, nodeIdA.String()))
		subs.Send(t.Context(), certResp)
		select {
		case <-msgCh:
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("haven't got log message, timeout")
		}
	})

	t.Run("send, one receiver", func(t *testing.T) {
		done := make(chan struct{})
		sender := func(ctx context.Context, msg any, receivers ...peer.ID) error {
			defer close(done)
			if assert.Len(t, receivers, 1) {
				assert.Equal(t, nodeIdA, receivers[0])
			}
			assert.Equal(t, certResp, msg)
			return nil
		}

		subs, err := NewSubscriptions(sender, observability.Default(t))
		require.NoError(t, err)
		require.NotNil(t, subs)

		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdA.String()))
		subs.Send(t.Context(), certResp)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("the send hasn't been called within timeout")
		}
		require.EqualValues(t, responsesPerSubscription-1, subs.subs[crKey][nodeIdA], "expected node A subscription counter to go down")
	})

	t.Run("send, two receivers", func(t *testing.T) {
		done := make(chan struct{})
		sender := func(ctx context.Context, msg any, receivers ...peer.ID) error {
			defer close(done)
			if assert.Len(t, receivers, 2) {
				assert.ElementsMatch(t, []peer.ID{nodeIdA, nodeIdB}, receivers)
			}
			assert.Equal(t, certResp, msg)
			return nil
		}

		subs, err := NewSubscriptions(sender, observability.Default(t))
		require.NoError(t, err)
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdA.String()))
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdB.String()))

		subs.Send(t.Context(), certResp)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("the send hasn't been called within timeout")
		}
		require.EqualValues(t, responsesPerSubscription-1, subs.subs[crKey][nodeIdA])
		require.EqualValues(t, responsesPerSubscription-1, subs.subs[crKey][nodeIdB])
	})

	t.Run("removed from registry", func(t *testing.T) {
		// node is subscribed for "responsesPerSubscription" calls so after that many
		// calls it's counter should be zero and next call will remove it from registry
		var sendCalls, receiverCnt atomic.Int32
		done := make(chan struct{})
		sender := func(ctx context.Context, msg any, receivers ...peer.ID) error {
			switch sendCalls.Add(1) {
			case responsesPerSubscription:
				done <- struct{}{}
			case responsesPerSubscription + 1:
				close(done)
			}
			receiverCnt.Add(int32(len(receivers)))
			return nil
		}

		subs, err := NewSubscriptions(sender, observability.Default(t))
		require.NoError(t, err)
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdA.String()))
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdB.String()))

		for range responsesPerSubscription {
			subs.Send(t.Context(), certResp)
		}
		select {
		case <-done:
		case <-time.After((responsesPerSubscription * 700) * time.Millisecond):
			t.Fatal("the send hasn't been called within timeout")
		}

		// both nodes should now be in the subscribers list with zero counter
		require.Len(t, subs.subs[crKey], 2, "subscribed nodes for the partition/shard")
		require.Contains(t, subs.subs[crKey], nodeIdA)
		require.EqualValues(t, 0, subs.subs[crKey][nodeIdA])
		require.Contains(t, subs.subs[crKey], nodeIdB)
		require.EqualValues(t, 0, subs.subs[crKey][nodeIdB])
		require.EqualValues(t, 2*responsesPerSubscription, receiverCnt.Load(), "all calls had 2 receivers")

		// next send should trigger removal of the subscription but resubscribe node B
		require.NoError(t, subs.Subscribe(certResp.Partition, certResp.Shard, nodeIdB.String()))
		subs.Send(t.Context(), certResp)
		select {
		case <-done:
		case <-time.After(1000 * time.Millisecond):
			t.Fatal("the send hasn't been called within timeout")
		}
		// A should be gone but B should have "initialCount-1" counter
		require.Len(t, subs.subs[crKey], 1, "subscribed nodes for the partition/shard")
		require.NotContains(t, subs.subs[crKey], nodeIdA, "expected that node A is removed")
		require.EqualValues(t, responsesPerSubscription-1, subs.subs[crKey][nodeIdB])
		require.EqualValues(t, (2*responsesPerSubscription)+1, receiverCnt.Load(), "last call had one receiver")
	})
}

func generateNodeID(t *testing.T) peer.ID {
	authSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	authVerifier, err := authSigner.Verifier()
	require.NoError(t, err)

	authKey, err := authVerifier.MarshalPublicKey()
	require.NoError(t, err)

	nodeID, err := network.NodeIDFromPublicKeyBytes(authKey)
	require.NoError(t, err)
	return nodeID
}
