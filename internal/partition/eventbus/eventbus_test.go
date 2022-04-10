package eventbus

import (
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/stretchr/testify/require"
)

func TestEventBus_New(t *testing.T) {
	bus := New()
	defer bus.Close()
	require.NotNil(t, bus)
	require.Equal(t, 0, len(bus.topics))
	require.False(t, bus.closing)
}

func TestSubscribe_EventBusClosing(t *testing.T) {
	bus := New()
	bus.Close()
	channel, err := bus.Subscribe("topic", 10)
	require.Error(t, err)
	require.ErrorIs(t, ErrEventBusClosing, err)
	require.Nil(t, channel)
}

func TestSubscribe_Ok(t *testing.T) {
	bus := New()
	defer bus.Close()
	channel, err := bus.Subscribe("topic", 10)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1, len(bus.topics))
	require.Equal(t, 10, cap(channel))
	require.Equal(t, 0, len(channel))
}

func TestSubmit_EventBusClosing(t *testing.T) {
	bus := New()
	bus.Close()
	err := bus.Submit("topic", "Hello!")
	require.Error(t, err)
	require.ErrorIs(t, ErrEventBusClosing, err)
}

func TestSubmit_TopicNotFound(t *testing.T) {
	bus := New()
	defer bus.Close()
	err := bus.Submit("topic", "Hello!")
	require.Error(t, err)
	require.ErrorIs(t, ErrTopicNotFound, err)
}

func TestSubmit_Ok(t *testing.T) {
	bus := New()
	defer bus.Close()
	channel, err := bus.Subscribe("topic", 10)
	require.NoError(t, err)
	err = bus.Submit("topic", "Hello!")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		m := <-channel
		return m.(string) == "Hello!"
	}, test.WaitDuration, test.WaitTick)
}

func TestClose_Ok(t *testing.T) {
	bus := New()
	bus.Subscribe("topic", 10)
	err := bus.Close()
	require.NoError(t, err)
	require.Equal(t, true, bus.closing)
}

func TestClose_AlreadyClosing(t *testing.T) {
	bus := New()
	bus.Subscribe("topic", 10)
	err := bus.Close()
	require.NoError(t, err)
	err = bus.Close()
	require.NoError(t, err)
}
