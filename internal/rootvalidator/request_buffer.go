package rootvalidator

import (
	"bytes"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
)

type CertRequestBuffer struct {
	store map[p.SystemIdentifier]*requestBuffer
}

// requestBuffer keeps track of received certification requests and counts state hashes.
type requestBuffer struct {
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// NewCertificationRequestBuffer create new certification requests buffer
func NewCertificationRequestBuffer() *CertRequestBuffer {
	return &CertRequestBuffer{
		store: make(map[p.SystemIdentifier]*requestBuffer),
	}
}

// Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
// equivocating and in both cases error is returned. Clear or Reset in order to receive new requests
func (c *CertRequestBuffer) Add(request *certification.BlockCertificationRequest) error {
	rs := c.get(p.SystemIdentifier(request.SystemIdentifier))
	return rs.add(request)
}

// GetRequests returns all stored requests per system identifier
func (c *CertRequestBuffer) GetRequests(id p.SystemIdentifier) []*certification.BlockCertificationRequest {
	rs := c.get(id)
	allReq := make([]*certification.BlockCertificationRequest, 0, len(rs.requests))
	for _, req := range rs.requests {
		allReq = append(allReq, req)
	}
	return allReq
}

// IsConsensusReceived has partition with id reached consensus. Required nrOfNodes as input to calculate consensus
func (c *CertRequestBuffer) IsConsensusReceived(partitionInfo partition_store.PartitionInfo) (*certificates.InputRecord, bool) {
	id := p.SystemIdentifier(partitionInfo.SystemDescription.SystemIdentifier)
	rs := c.get(id)
	return rs.isConsensusReceived(partitionInfo)
}

// get returns an existing store for system identifier or registers and returns a new one if none existed
func (c *CertRequestBuffer) get(id p.SystemIdentifier) *requestBuffer {
	rs, f := c.store[id]
	if !f {
		rs = newRequestStore()
		c.store[id] = rs
	}
	return rs
}

// Reset removed all incoming requests from all stores
func (c *CertRequestBuffer) Reset() {
	for _, rs := range c.store {
		rs.reset()
	}
}

// Clear clears requests in one partition
func (c *CertRequestBuffer) Clear(id p.SystemIdentifier) {
	rs := c.get(id)
	logger.Debug("Resetting request store for partition '%X'", id)
	rs.reset()
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
			logger.Warning("Equivocating request with different hash: %v", req)
			return errors.New("equivocating request with different hash")
		} else {
			logger.Debug("Duplicated request: %v", req)
			return errors.New("duplicated request")
		}
	}
	// no request in buffered yet, add
	hashString := string(req.InputRecord.Hash)
	rs.requests[req.NodeIdentifier] = req
	count := rs.hashCounts[hashString]
	rs.hashCounts[hashString] = count + 1
	logger.Debug("Added new IR hash: %X, block hash: %X, total hash count: %v", req.InputRecord.Hash, req.InputRecord.BlockHash, rs.hashCounts[hashString])
	return nil
}

func (rs *requestBuffer) reset() {
	rs.requests = make(map[string]*certification.BlockCertificationRequest)
	rs.hashCounts = make(map[string]uint)
}

func (rs *requestBuffer) isConsensusReceived(partition partition_store.PartitionInfo) (*certificates.InputRecord, bool) {
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
		logger.Debug("Consensus possible in the future, no hashes received yet")
		return nil, true
	}
	quorum := partition.GetQuorum()
	if uint64(c) >= quorum {
		// consensus received
		for _, req := range rs.requests {
			if bytes.Equal(h, req.InputRecord.Hash) {
				logger.Debug("Consensus achieved, returning IR (hash: %X, block hash: %X)", req.InputRecord.Hash, req.InputRecord.BlockHash)
				return req.InputRecord, true
			}
		}
	} else if len(partition.TrustBase)-len(rs.requests)+int(c) < int(quorum) {
		logger.Debug("Consensus not possible, hash count: %v, needed count: %v, missing: %v", c, quorum, len(partition.TrustBase)-len(rs.requests))
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	logger.Debug("Consensus possible in the future, hash count: %v, needed count: %v, hash:%X", c, quorum, h)
	return nil, true
}

func (rs *requestBuffer) remove(nodeId string) {
	oldReq, f := rs.requests[nodeId]
	if !f {
		return
	}
	hashString := string(oldReq.InputRecord.Hash)
	rs.hashCounts[hashString] = rs.hashCounts[hashString] - 1
	logger.Debug("Removing old IR hash: %X, block hash: %X, new hash count: %v", oldReq.InputRecord.Hash, oldReq.InputRecord.BlockHash, rs.hashCounts[hashString])
	delete(rs.requests, nodeId)
}
