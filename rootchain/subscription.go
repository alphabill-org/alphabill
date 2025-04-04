package rootchain

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/observability"
)

const responsesPerSubscription = 2

type Subscriptions struct {
	mu     sync.RWMutex
	subs   map[partitionShard]map[peer.ID]int
	sender func(ctx context.Context, msg any, receivers ...peer.ID) error

	log *slog.Logger

	bcRespSent metric.Int64Counter // number of Block Certification Responses sent
}

func NewSubscriptions(sender func(ctx context.Context, msg any, receivers ...peer.ID) error, obs Observability) (*Subscriptions, error) {
	r := &Subscriptions{
		subs:   map[partitionShard]map[peer.ID]int{},
		sender: sender,
		log:    obs.Logger(),
	}

	meter := obs.Meter("rootchain.node")

	var err error
	r.bcRespSent, err = meter.Int64Counter("block.cert.rsp", metric.WithDescription("Number of Block Certification Responses sent (ie how many subscribers the node had)"))
	if err != nil {
		return nil, fmt.Errorf("creating Block Certification Responses counter: %w", err)
	}

	return r, nil
}

func (s *Subscriptions) Subscribe(partition types.PartitionID, shard types.ShardID, nodeId string) error {
	peerID, err := peer.Decode(nodeId)
	if err != nil {
		return fmt.Errorf("invalid receiver id: %w", err)
	}
	id := partitionShard{partition: partition, shard: shard.Key()}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, found := s.subs[id]; !found {
		s.subs[id] = make(map[peer.ID]int)
	}
	s.subs[id][peerID] = responsesPerSubscription
	return nil
}

func (s *Subscriptions) Send(ctx context.Context, cr *certification.CertificationResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs := s.subs[partitionShard{cr.Partition, cr.Shard.Key()}]
	recipients := make([]peer.ID, 0, len(subs))
	for peerID, cnt := range subs {
		if cnt == 0 {
			delete(subs, peerID)
			continue
		}

		subs[peerID]--
		recipients = append(recipients, peerID)
	}

	go func() {
		s.log.DebugContext(ctx, fmt.Sprintf("sending CertificationResponse, %d receivers, R_next: %d, IR Hash: %X, Block Hash: %X",
			len(recipients), cr.Technical.Round, cr.UC.InputRecord.Hash, cr.UC.InputRecord.BlockHash), logger.Shard(cr.Partition, cr.Shard))
		if len(recipients) > 0 {
			if err := s.sender(ctx, cr, recipients...); err != nil {
				s.log.WarnContext(ctx, "sending certification result", logger.Error(err), logger.Shard(cr.Partition, cr.Shard))
			}
			s.bcRespSent.Add(ctx, int64(len(recipients)), observability.Shard(cr.Partition, cr.Shard))
		}
	}()
}
