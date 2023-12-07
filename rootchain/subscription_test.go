package rootchain

import (
	"testing"

	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestNewSubscriptionsEmpty(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, subscriptions)
	// no panic
	var sysID1 types.SystemID32 = 1
	subscriptions.SubscriberError(sysID1, "1")
	require.Len(t, subscriptions.subs, 0)
	subscribed := subscriptions.Get(sysID1)
	require.Len(t, subscribed, 0)
}

func TestNewSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions()
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
	subscribed = subscriptions.Get(types.SystemID32(3))
	require.Len(t, subscribed, 0)
}

func TestExpiredSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, subscriptions)
	subscriptions.Subscribe(sysID1, "1")
	subscriptions.Subscribe(sysID1, "2")
	// subscription is removed on errorCount number of send errors
	for i := 0; i < defaultSubscriptionErrorCount; i++ {
		subscriptions.SubscriberError(sysID1, "1")
	}
	subscribed := subscriptions.Get(sysID1)
	require.Len(t, subscribed, 1)
}

func TestSubscriptionRefresh(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, subscriptions)
	subscriptions.Subscribe(sysID1, "1")
	subscriptions.Subscribe(sysID1, "2")
	// subscription is removed on errorCount number of send errors
	for i := 1; i < defaultSubscriptionErrorCount; i++ {
		subscriptions.SubscriberError(sysID1, "1")
	}
	subscribed := subscriptions.Get(sysID1)
	require.Len(t, subscribed, 2)
	// simulate new request from partition
	subscriptions.Subscribe(sysID1, "1")
	require.Equal(t, defaultSubscriptionErrorCount, subscriptions.subs[sysID1]["1"])
}
