package rootvalidator

import (
	"sync"

	p "github.com/alphabill-org/alphabill/internal/network/protocol"
)

const defaultSubscriptionErrorCount = 3

type Subscriptions struct {
	mu   sync.RWMutex
	subs map[p.SystemIdentifier]map[string]int
}

func NewSubscriptions() Subscriptions {
	return Subscriptions{
		subs: map[p.SystemIdentifier]map[string]int{},
	}
}

func (s Subscriptions) Subscribe(id p.SystemIdentifier, nodeId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.subs[id]
	if found == false {
		s.subs[id] = make(map[string]int)
	}
	s.subs[id][nodeId] = defaultSubscriptionErrorCount
}

func (s Subscriptions) SubscriberError(id p.SystemIdentifier, nodeId string) {
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

func (s Subscriptions) Get(id p.SystemIdentifier) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subs := s.subs[id]
	subscribed := make([]string, 0, len(subs))
	for k := range subs {
		subscribed = append(subscribed, k)
	}
	return subscribed
}
