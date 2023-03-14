package rootvalidator

import p "github.com/alphabill-org/alphabill/internal/network/protocol"

const defaultSubscriptionErrorCount = 3

type Subscriptions map[p.SystemIdentifier]map[string]int

func NewSubscriptions() Subscriptions {
	return Subscriptions{}
}

func (s Subscriptions) Subscribe(id p.SystemIdentifier, nodeId string) {
	_, found := s[id]
	if found == false {
		s[id] = make(map[string]int)
	}
	s[id][nodeId] = defaultSubscriptionErrorCount
}

func (s Subscriptions) SubscriberError(id p.SystemIdentifier, nodeId string) {
	subs, found := s[id]
	if !found {
		return
	}
	subs[nodeId]--
	if subs[nodeId] <= 0 {
		delete(subs, nodeId)
	}
}

func (s Subscriptions) Get(id p.SystemIdentifier) []string {
	subs := s[id]
	subscribed := make([]string, 0, len(subs))
	for k := range subs {
		subscribed = append(subscribed, k)
	}
	return subscribed
}
