package rootchain

import (
	"sync"

	"github.com/alphabill-org/alphabill/types"
)

const defaultSubscriptionErrorCount = 3

type Subscriptions struct {
	mu   sync.RWMutex
	subs map[types.SystemID32]map[string]int
}

func NewSubscriptions() *Subscriptions {
	return &Subscriptions{
		subs: map[types.SystemID32]map[string]int{},
	}
}

func (s *Subscriptions) Subscribe(id types.SystemID32, nodeId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.subs[id]
	if !found {
		s.subs[id] = make(map[string]int)
	}
	s.subs[id][nodeId] = defaultSubscriptionErrorCount
}

func (s *Subscriptions) SubscriberError(id types.SystemID32, nodeId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs, found := s.subs[id]
	if !found {
		return
	}
	subs[nodeId]--
	if subs[nodeId] <= 0 {
		delete(subs, nodeId)
	}
}

func (s *Subscriptions) Get(id types.SystemID32) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subs := s.subs[id]
	subscribed := make([]string, 0, len(subs))
	for k := range subs {
		subscribed = append(subscribed, k)
	}
	return subscribed
}
