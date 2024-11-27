package rootchain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
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
func NewCertificationRequestBuffer() *CertRequestBuffer {
	return &CertRequestBuffer{
		store: make(map[partitionShard]*requestBuffer),
	}
}

// Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
// equivocating and in both cases error is returned. Clear or Reset in order to receive new nodeRequest
func (c *CertRequestBuffer) Add(request *certification.BlockCertificationRequest, tb QuorumInfo) (QuorumStatus, []*certification.BlockCertificationRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rs := c.get(request.Partition, request.Shard)
	return rs.add(request, tb)
}

// IsConsensusReceived has partition with id reached consensus
func (c *CertRequestBuffer) IsConsensusReceived(id types.PartitionID, shard types.ShardID, tb QuorumInfo) QuorumStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.get(id, shard)
	_, res := rs.isConsensusReceived(tb)
	return res
}

// Reset removed all incoming nodeRequest from all stores
func (c *CertRequestBuffer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range c.store {
		rs.reset()
	}
}

// Clear clears node request in one partition
func (c *CertRequestBuffer) Clear(id types.PartitionID, shard types.ShardID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rs := c.get(id, shard)
	rs.reset()
}

// get returns an existing store for partition identifier or registers and returns a new one if none existed
func (c *CertRequestBuffer) get(id types.PartitionID, shard types.ShardID) *requestBuffer {
	key := partitionShard{partition: id, shard: shard.Key()}
	rs, f := c.store[key]
	if !f {
		rs = newRequestStore()
		c.store[key] = rs
	}
	return rs
}

// newRequestStore creates a new empty requestBuffer.
func newRequestStore() *requestBuffer {
	s := &requestBuffer{
		nodeRequest: make(map[string]struct{}),
		requests:    make(map[sha256Hash][]*certification.BlockCertificationRequest),
	}
	return s
}

// add stores a new input record received from the node.
func (rs *requestBuffer) add(req *certification.BlockCertificationRequest, tb QuorumInfo) (QuorumStatus, []*certification.BlockCertificationRequest, error) {
	if _, f := rs.nodeRequest[req.NodeIdentifier]; f {
		return QuorumUnknown, nil, errors.New("request of the node in this round already stored")
	}

	irBytes, err := req.InputRecord.Bytes()
	if err != nil {
		return QuorumUnknown, nil, fmt.Errorf("new certificate input record bytes: %w", err)
	}
	hash := sha256.Sum256(irBytes)
	rs.nodeRequest[req.NodeIdentifier] = struct{}{}
	rs.requests[hash] = append(rs.requests[hash], req)
	proof, res := rs.isConsensusReceived(tb)
	return res, proof, nil
}

func (rs *requestBuffer) reset() {
	clear(rs.nodeRequest)
	clear(rs.requests)
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
