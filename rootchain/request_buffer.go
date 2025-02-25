package rootchain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/observability"
)

type (
	QuorumStatus uint8

	QuorumInfo interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
	}

	CertRequestBuffer struct {
		mu    sync.RWMutex
		store map[partitionShard]*requestBuffer

		consensusDur metric.Float64Histogram
		responseDur  metric.Float64Histogram
	}

	partitionShard struct {
		partition types.PartitionID
		shard     string // can't use ShardID type as it's not comparable
	}

	sha256Hash [sha256.Size]byte

	// requestBuffer keeps track of received Certification Request
	requestBuffer struct {
		// index of nodes which have voted (key is node identifier)
		nodeRequest map[string]struct{}
		// index to count votes, key is IR record hash
		requests map[sha256Hash][]*certification.BlockCertificationRequest
		qState   QuorumStatus

		start     time.Time
		attrShard metric.MeasurementOption
	}
)

const (
	QuorumUnknown QuorumStatus = iota
	QuorumInProgress
	QuorumAchieved
	QuorumNotPossible
)

func (qs QuorumStatus) String() string {
	switch qs {
	case QuorumInProgress:
		return "QuorumInProgress"
	case QuorumAchieved:
		return "QuorumAchieved"
	case QuorumNotPossible:
		return "QuorumNotPossible"
	case QuorumUnknown:
		return "QuorumUnknown"
	default:
		return fmt.Sprintf("QuorumStatus(%d)", int(qs))
	}
}

// NewCertificationRequestBuffer create new certification nodeRequest buffer
func NewCertificationRequestBuffer(m metric.Meter) (*CertRequestBuffer, error) {
	consensusDur, err := m.Float64Histogram("cert.req.consensus.time",
		metric.WithDescription("How long it took to achieve consensus about shard's certification request, ie enough shard nodes have sent in their take for the round"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(200e-6, 400e-6, 800e-6, 0.0016, 0.003, 0.006, 0.01, 0.05, 0.1, 0.2, 0.4))
	if err != nil {
		return nil, fmt.Errorf("creating histogram for req consensus time: %w", err)
	}
	responseDur, err := m.Float64Histogram("cert.rsp.ready.time",
		metric.WithDescription("How long it took from handing certification request over to consensus manager to sending out certification result"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.1, 1.5))
	if err != nil {
		return nil, fmt.Errorf("creating histogram for cert ready time: %w", err)
	}

	return &CertRequestBuffer{
		store:        make(map[partitionShard]*requestBuffer),
		consensusDur: consensusDur,
		responseDur:  responseDur,
	}, nil
}

/*
Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
equivocating and in both cases error is returned. Clear in order to receive new nodeRequest (ie to start
collecting requests for the next round).
*/
func (c *CertRequestBuffer) Add(ctx context.Context, request *certification.BlockCertificationRequest, tb QuorumInfo) (QuorumStatus, []*certification.BlockCertificationRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs := c.get(request.PartitionID, request.ShardID)
	qs, bcr, err := rs.add(request, tb)
	if err == nil {
		c.updQuorumStatus(ctx, rs, qs)
	}
	return qs, bcr, err
}

func (c *CertRequestBuffer) IsConsensusReceived(partition types.PartitionID, shard types.ShardID, tb QuorumInfo) QuorumStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.get(partition, shard)
	return rs.qState
}

/*
Clear clears node request in one shard - this must be called when the shard's Certification Request for a
round has been processed in order for the buffer to accept requests for the next round.
*/
func (c *CertRequestBuffer) Clear(ctx context.Context, partition types.PartitionID, shard types.ShardID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs := c.get(partition, shard)
	switch rs.qState {
	case QuorumAchieved, QuorumNotPossible:
		c.responseDur.Record(ctx, time.Since(rs.start).Seconds())
	}
	rs.reset()
}

// get returns an existing store for shard or registers and returns a new one if none existed
func (c *CertRequestBuffer) get(partition types.PartitionID, shard types.ShardID) *requestBuffer {
	key := partitionShard{partition: partition, shard: shard.Key()}
	rs, f := c.store[key]
	if !f {
		rs = newRequestStore()
		rs.attrShard = observability.Shard(partition, shard)
		c.store[key] = rs
	}
	return rs
}

func (c *CertRequestBuffer) updQuorumStatus(ctx context.Context, rb *requestBuffer, qs QuorumStatus) {
	if rb.qState == qs {
		return
	}

	switch qs {
	case QuorumAchieved:
		c.consensusDur.Record(ctx, time.Since(rb.start).Seconds(), attrQuorumAchieved, rb.attrShard)
	case QuorumNotPossible:
		c.consensusDur.Record(ctx, time.Since(rb.start).Seconds(), attrQuorumNotPossible)
	}
	rb.qState = qs
	// start clock to track time until Clear is called (consensus has been certified)
	rb.start = time.Now()
}

// newRequestStore creates a new empty requestBuffer.
func newRequestStore() *requestBuffer {
	s := &requestBuffer{
		nodeRequest: make(map[string]struct{}),
		requests:    make(map[sha256Hash][]*certification.BlockCertificationRequest),
		qState:      QuorumInProgress,
	}
	return s
}

// add stores a new input record received from the node.
func (rs *requestBuffer) add(req *certification.BlockCertificationRequest, tb QuorumInfo) (QuorumStatus, []*certification.BlockCertificationRequest, error) {
	if _, f := rs.nodeRequest[req.NodeID]; f {
		return QuorumUnknown, nil, errors.New("request of the node in this round already stored")
	}
	if len(rs.nodeRequest) == 0 {
		// start clock to track time until consensus is achieved
		rs.start = time.Now()
	}

	irBytes, err := req.InputRecord.Bytes()
	if err != nil {
		return QuorumUnknown, nil, fmt.Errorf("reading input record bytes: %w", err)
	}
	hash := sha256.Sum256(irBytes)
	rs.nodeRequest[req.NodeID] = struct{}{}
	rs.requests[hash] = append(rs.requests[hash], req)
	proof, res := rs.isConsensusReceived(tb)
	return res, proof, nil
}

func (rs *requestBuffer) reset() {
	clear(rs.nodeRequest)
	clear(rs.requests)
	rs.qState = QuorumInProgress
}

func (rs *requestBuffer) isConsensusReceived(tb QuorumInfo) ([]*certification.BlockCertificationRequest, QuorumStatus) {
	// find most voted IR
	votes := 0
	var bcReqs []*certification.BlockCertificationRequest
	for _, reqs := range rs.requests {
		if votes < len(reqs) {
			votes = len(reqs)
			bcReqs = reqs
		}
	}

	quorum := int(tb.GetQuorum())
	if votes >= quorum {
		return bcReqs, QuorumAchieved
	}

	if int(tb.GetTotalNodes())-len(rs.nodeRequest)+votes < quorum {
		// enough nodes have voted and even if the rest of the votes are for
		// the most popular option, quorum is still not possible
		allReq := make([]*certification.BlockCertificationRequest, 0, len(rs.nodeRequest))
		for _, req := range rs.requests {
			allReq = append(allReq, req...)
		}
		return allReq, QuorumNotPossible
	}

	// consensus possible in the future
	return nil, QuorumInProgress
}

var (
	attrQuorumAchieved    = metric.WithAttributeSet(attribute.NewSet(attribute.Int("status", int(QuorumAchieved))))
	attrQuorumNotPossible = metric.WithAttributeSet(attribute.NewSet(attribute.Int("status", int(QuorumNotPossible))))
)
