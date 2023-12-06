package testevent

import (
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/testutils"
	"github.com/stretchr/testify/require"
)

type TestEventHandler struct {
	mutex  sync.Mutex
	events []*event.Event
}

func (eh *TestEventHandler) HandleEvent(e *event.Event) {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	eh.events = append(eh.events, e)
}

func (eh *TestEventHandler) GetEvents() []*event.Event {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	return eh.events
}

func (eh *TestEventHandler) Reset() {
	eh.mutex.Lock()
	defer eh.mutex.Unlock()
	eh.events = []*event.Event{}
}

func ContainsEvent(t *testing.T, eh *TestEventHandler, et event.Type) {
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

func NotContainsEvent(t *testing.T, eh *TestEventHandler, et event.Type) {
	events := eh.GetEvents()
	for _, e := range events {
		if e.EventType == et {
			t.Errorf("event %v should not be present", et)
		}
	}
}
