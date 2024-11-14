package rootchain

import (
	"context"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const responsesPerSubscription = 2

type Subscriptions struct {
	mu   sync.RWMutex
	subs map[types.PartitionID]map[string]int
}

func NewSubscriptions(m metric.Meter) *Subscriptions {
	r := &Subscriptions{
		subs: map[types.PartitionID]map[string]int{},
	}

	_, _ = m.Int64ObservableUpDownCounter("subscriptions",
		metric.WithDescription("number of responses to send to subscribed validators"),
		metric.WithInt64Callback(
			func(ctx context.Context, io metric.Int64Observer) error {
				r.mu.Lock()
				defer r.mu.Unlock()

				for sysID, nodes := range r.subs {
					partition := observability.Partition(sysID)
					for nodeID, cnt := range nodes {
						io.Observe(int64(cnt), metric.WithAttributes(partition, attribute.String("node.id", nodeID)))
					}
				}

				return nil
			},
		))

	return r
}

func (s *Subscriptions) Subscribe(id types.PartitionID, nodeId string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, found := s.subs[id]; !found {
		s.subs[id] = make(map[string]int)
	}
	s.subs[id][nodeId] = responsesPerSubscription
}

func (s *Subscriptions) ResponseSent(id types.PartitionID, nodeId string) {
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

func (s *Subscriptions) Get(id types.PartitionID) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	subs := s.subs[id]
	subscribed := make([]string, 0, len(subs))
	for k := range subs {
		subscribed = append(subscribed, k)
	}
	return subscribed
}
