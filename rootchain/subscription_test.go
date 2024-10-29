package rootchain

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewSubscriptionsEmpty(t *testing.T) {
	subscriptions := NewSubscriptions(noop.NewMeterProvider().Meter(t.Name()))
	require.NotNil(t, subscriptions)
	// no panic
	var sysID1 types.PartitionID = 1
	subscriptions.ResponseSent(sysID1, "1")
	require.Empty(t, subscriptions.subs)
	subscribed := subscriptions.Get(sysID1)
	require.Empty(t, subscribed)
}

func TestNewSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions(noop.NewMeterProvider().Meter(t.Name()))
	require.NotNil(t, subscriptions)
	subscriptions.Subscribe(sysID1, "1")
	subscriptions.Subscribe(sysID1, "2")
	subscriptions.Subscribe(sysID2, "1")
	subscriptions.Subscribe(sysID2, "1")
	subscribed := subscriptions.Get(sysID1)
	require.Len(t, subscribed, 2)
	require.Contains(t, subscribed, "1")
	require.Contains(t, subscribed, "2")
	subscribed = subscriptions.Get(sysID2)
	require.Contains(t, subscribed, "1")
	require.NotContains(t, subscribed, "2")
	subscribed = subscriptions.Get(3)
	require.Empty(t, subscribed)
}

func TestExpiredSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions(noop.NewMeterProvider().Meter(t.Name()))
	require.NotNil(t, subscriptions)
	subscriptions.Subscribe(sysID1, "1")
	subscriptions.Subscribe(sysID1, "2")
	// subscription is decremented on each send attempt
	for i := 0; i < responsesPerSubscription; i++ {
		subscriptions.ResponseSent(sysID1, "1")
	}
	subscribed := subscriptions.Get(sysID1)
	// two responses where sent to "1" and it is now removed from subscriptions
	require.NotContains(t, subscribed, "1")
	// node 2 is still there
	require.Contains(t, subscribed, "2")
}

func TestSubscriptionRefresh(t *testing.T) {
	subscriptions := NewSubscriptions(noop.NewMeterProvider().Meter(t.Name()))
	require.NotNil(t, subscriptions)
	subscriptions.Subscribe(sysID1, "1")
	subscriptions.Subscribe(sysID1, "2")
	// subscription is decremented on each send attempt
	for i := 1; i < responsesPerSubscription; i++ {
		subscriptions.ResponseSent(sysID1, "1")
	}
	subscribed := subscriptions.Get(sysID1)
	require.Len(t, subscribed, 2)
	// simulate new request from partition
	subscriptions.Subscribe(sysID1, "1")
	require.Equal(t, responsesPerSubscription, subscriptions.subs[sysID1]["1"])
}
