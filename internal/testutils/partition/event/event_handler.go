package testevent

import (
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/partition"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

type TestEventHandler struct {
	mutex  sync.Mutex
	events []*partition.Event
}

func (eh *TestEventHandler) HandleEvent(e *partition.Event) {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	eh.events = append(eh.events, e)
}

func (eh *TestEventHandler) GetEvents() []*partition.Event {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	return eh.events
}

func (eh *TestEventHandler) Reset() {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	eh.events = []*partition.Event{}
}

func ContainsEvent(t *testing.T, eh *TestEventHandler, et partition.EventType) {
	require.Eventually(t, func() bool {
		events := eh.GetEvents()
		for _, e := range events {
			if e.EventType == et {
				return true
			}
		}
		return false

	}, test.WaitDuration, test.WaitTick)
}
