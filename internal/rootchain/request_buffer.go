package rootchain

import (
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	CertRequestBuffer struct {
		mu    sync.RWMutex
		store map[protocol.SystemIdentifier]*requestBuffer
	}

	// requestBuffer keeps track of received certification nodeRequest and counts state hashes.
	sha256Hash    [sha256.Size]byte
	requestBuffer struct {
		nodeRequest map[string]sha256Hash                                     // node request register - one request per node id per round (key is node identifier)
		requests    map[sha256Hash][]*certification.BlockCertificationRequest // unique requests, IR record hash to certification request
	}
)

// NewCertificationRequestBuffer create new certification nodeRequest buffer
func NewCertificationRequestBuffer() *CertRequestBuffer {
	return &CertRequestBuffer{
		store: make(map[protocol.SystemIdentifier]*requestBuffer),
	}
}

// Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
// equivocating and in both cases error is returned. Clear or Reset in order to receive new nodeRequest
func (c *CertRequestBuffer) Add(request *certification.BlockCertificationRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	rs := c.get(protocol.SystemIdentifier(request.SystemIdentifier))
	return rs.add(request)
}

// GetRequests returns all stored nodeRequest per system identifier
func (c *CertRequestBuffer) GetRequests(id protocol.SystemIdentifier) []*certification.BlockCertificationRequest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.get(id)
	allReq := make([]*certification.BlockCertificationRequest, 0, len(rs.nodeRequest))
	for _, req := range rs.requests {
		allReq = append(allReq, req...)
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

// Reset removed all incoming nodeRequest from all stores
func (c *CertRequestBuffer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range c.store {
		rs.reset()
	}
}

// Clear clears nodeRequest in one partition
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
		nodeRequest: make(map[string]sha256Hash),
		requests:    make(map[sha256Hash][]*certification.BlockCertificationRequest),
	}
	return s
}

// add stores a new input record received from the node.
func (rs *requestBuffer) add(req *certification.BlockCertificationRequest) error {
	_, f := rs.nodeRequest[req.NodeIdentifier]
	if f {
		return errors.New("request in this round already stored, rejected")
	}
	// first request, calculate hash of IR and store
	hash := sha256.Sum256(req.InputRecord.Bytes())
	// no request in buffered yet, add
	rs.nodeRequest[req.NodeIdentifier] = hash
	rs.requests[hash] = append(rs.requests[hash], req)
	return nil
}

func (rs *requestBuffer) reset() {
	rs.nodeRequest = make(map[string]sha256Hash)
	rs.requests = make(map[sha256Hash][]*certification.BlockCertificationRequest)
}

func (rs *requestBuffer) isConsensusReceived(tb partitions.PartitionTrustBase) (*types.InputRecord, bool) {
	var (
		reqCount = 0
		irHash   []byte
		bcReqs   []*certification.BlockCertificationRequest = nil
	)
	// find IR hash that has with most matching requests
	for hash, reqs := range rs.requests {
		if reqCount < len(reqs) {
			reqCount = len(reqs)
			irHash = hash[:]
			bcReqs = reqs
		}
	}
	quorum := int(tb.GetQuorum())
	if reqCount >= quorum {
		// consensus received
		return bcReqs[0].InputRecord, true
	}
	// enough nodes have voted and even if the rest of the votes are for the most popular, quorum is still not possible
	if int(tb.GetTotalNodes())-len(rs.nodeRequest)+reqCount < quorum {
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	logger.Trace("Consensus possible in the future, hash count: %v, needed count: %v, hash:%X", reqCount, quorum, irHash)
	return nil, true
}
