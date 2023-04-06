package rootchain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSubscriptionsEmpty(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, &subscriptions)
	// no panic
	subscriptions.SubscriberError("foo", "1")
	require.Len(t, subscriptions.subs, 0)
	subscribed := subscriptions.Get("foo")
	require.Len(t, subscribed, 0)
}

func TestNewSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, &subscriptions)
	subscriptions.Subscribe("foo", "1")
	subscriptions.Subscribe("foo", "2")
	subscriptions.Subscribe("bar", "1")
	subscriptions.Subscribe("bar", "1")
	subscribed := subscriptions.Get("foo")
	require.Len(t, subscribed, 2)
	require.Contains(t, subscribed, "1")
	require.Contains(t, subscribed, "2")
	subscribed = subscriptions.Get("bar")
	require.Contains(t, subscribed, "1")
	require.NotContains(t, subscribed, "2")
	subscribed = subscriptions.Get("brr")
	require.Len(t, subscribed, 0)
}

func TestExpiredSubscriptions(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, &subscriptions)
	subscriptions.Subscribe("foo", "1")
	subscriptions.Subscribe("foo", "2")
	// subscription is removed on errorCount number of send errors
	for i := 0; i < defaultSubscriptionErrorCount; i++ {
		subscriptions.SubscriberError("foo", "1")
	}
	subscribed := subscriptions.Get("foo")
	require.Len(t, subscribed, 1)
}

func TestSubscriptionRefresh(t *testing.T) {
	subscriptions := NewSubscriptions()
	require.NotNil(t, &subscriptions)
	subscriptions.Subscribe("foo", "1")
	subscriptions.Subscribe("foo", "2")
	// subscription is removed on errorCount number of send errors
	for i := 1; i < defaultSubscriptionErrorCount; i++ {
		subscriptions.SubscriberError("foo", "1")
	}
	subscribed := subscriptions.Get("foo")
	require.Len(t, subscribed, 2)
	// simulate new request from partition
	subscriptions.Subscribe("foo", "1")
	require.Equal(t, defaultSubscriptionErrorCount, subscriptions.subs["foo"]["1"])
}
