package rootchain

import (
	"bytes"
	"errors"
	"sync"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
)

type CertRequestBuffer struct {
	mu    sync.RWMutex
	store map[protocol.SystemIdentifier]*requestBuffer
}

// requestBuffer keeps track of received certification requests and counts state hashes.
type requestBuffer struct {
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// NewCertificationRequestBuffer create new certification requests buffer
func NewCertificationRequestBuffer() *CertRequestBuffer {
	return &CertRequestBuffer{
		store: make(map[protocol.SystemIdentifier]*requestBuffer),
	}
}

// Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
// equivocating and in both cases error is returned. Clear or Reset in order to receive new requests
func (c *CertRequestBuffer) Add(request *certification.BlockCertificationRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	rs := c.get(protocol.SystemIdentifier(request.SystemIdentifier))
	return rs.add(request)
}

// GetRequests returns all stored requests per system identifier
func (c *CertRequestBuffer) GetRequests(id protocol.SystemIdentifier) []*certification.BlockCertificationRequest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.get(id)
	allReq := make([]*certification.BlockCertificationRequest, 0, len(rs.requests))
	for _, req := range rs.requests {
		allReq = append(allReq, req)
	}
	return allReq
}

// IsConsensusReceived has partition with id reached consensus. Required nrOfNodes as input to calculate consensus
func (c *CertRequestBuffer) IsConsensusReceived(id protocol.SystemIdentifier, tb partitions.PartitionTrustBase) (*types.InputRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.get(id)
	return rs.isConsensusReceived(tb)
}

// Reset removed all incoming requests from all stores
func (c *CertRequestBuffer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range c.store {
		rs.reset()
	}
}

// Clear clears requests in one partition
func (c *CertRequestBuffer) Clear(id protocol.SystemIdentifier) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rs := c.get(id)
	rs.reset()
}

// get returns an existing store for system identifier or registers and returns a new one if none existed
func (c *CertRequestBuffer) get(id protocol.SystemIdentifier) *requestBuffer {
	rs, f := c.store[id]
	if !f {
		rs = newRequestStore()
		c.store[id] = rs
	}
	return rs
}

// newRequestStore creates a new empty requestBuffer.
func newRequestStore() *requestBuffer {
	s := &requestBuffer{
		requests:   make(map[string]*certification.BlockCertificationRequest),
		hashCounts: make(map[string]uint)}
	return s
}

// add stores a new input record received from the node.
func (rs *requestBuffer) add(req *certification.BlockCertificationRequest) error {
	prevReq, f := rs.requests[req.NodeIdentifier]
	if f {
		// Partition node tries to certify and equivocating request
		if !bytes.Equal(prevReq.InputRecord.Hash, req.InputRecord.Hash) {
			return errors.New("equivocating request with different hash")
		} else {
			return errors.New("duplicated request")
		}
	}
	// no request in buffered yet, add
	hashString := string(req.InputRecord.Hash)
	rs.requests[req.NodeIdentifier] = req
	count := rs.hashCounts[hashString]
	rs.hashCounts[hashString] = count + 1
	return nil
}

func (rs *requestBuffer) reset() {
	rs.requests = make(map[string]*certification.BlockCertificationRequest)
	rs.hashCounts = make(map[string]uint)
}

func (rs *requestBuffer) isConsensusReceived(tb partitions.PartitionTrustBase) (*types.InputRecord, bool) {
	var h []byte
	var c uint = 0
	for hash, count := range rs.hashCounts {
		if c < count {
			c = count
			h = []byte(hash)
		}
	}
	if h == nil {
		// consensus possible in the future
		logger.Trace("Consensus possible in the future, no hashes received yet")
		return nil, true
	}
	quorum := int(tb.GetQuorum())
	if int(c) >= quorum {
		// consensus received
		for _, req := range rs.requests {
			if bytes.Equal(h, req.InputRecord.Hash) {
				return req.InputRecord, true
			}
		}
	} else if int(tb.GetTotalNodes())-len(rs.requests)+int(c) < quorum {
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	logger.Trace("Consensus possible in the future, hash count: %v, needed count: %v, hash:%X", c, quorum, h)
	return nil, true
}
