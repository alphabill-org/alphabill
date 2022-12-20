package rootchain

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

type CertRequestStore struct {
	store map[p.SystemIdentifier]*requestStore
}

// requestStore keeps track of received certification requests and counts state hashes.
type requestStore struct {
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// NewCertificationRequestStore create new certification requests store
func NewCertificationRequestStore() *CertRequestStore {
	return &CertRequestStore{
		store: make(map[p.SystemIdentifier]*requestStore),
	}
}

// Add request to certification store. Per node id first valid request is stored. Rest are either duplicate or
// equivocating and in both cases error is returned. Clear or Reset in order to receive new requests
func (c *CertRequestStore) Add(request *certification.BlockCertificationRequest) error {
	rs := c.get(p.SystemIdentifier(request.SystemIdentifier))
	return rs.add(request)
}

// GetRequests returns all stored requests per system identifier
func (c *CertRequestStore) GetRequests(id p.SystemIdentifier) []*certification.BlockCertificationRequest {
	rs := c.get(id)
	allReq := make([]*certification.BlockCertificationRequest, 0, len(rs.requests))
	for _, req := range rs.requests {
		allReq = append(allReq, req)
	}
	return allReq
}

// IsConsensusReceived has partition with id reached consensus. Required nrOfNodes as input to calculate consensus
func (c *CertRequestStore) IsConsensusReceived(id p.SystemIdentifier, nrOfNodes int) (*certificates.InputRecord, bool) {
	rs := c.get(id)
	return rs.isConsensusReceived(nrOfNodes)
}

// get returns an existing store for system identifier or registers and returns a new one if none existed
func (c *CertRequestStore) get(id p.SystemIdentifier) *requestStore {
	rs, f := c.store[id]
	if !f {
		rs = newRequestStore()
		c.store[id] = rs
	}
	return rs
}

// Reset removed all incoming requests from all stores
func (c *CertRequestStore) Reset() {
	for _, rs := range c.store {
		rs.reset()
	}
}

// Clear clears requests in one partition
func (c *CertRequestStore) Clear(id p.SystemIdentifier) {
	rs := c.get(id)
	logger.Debug("Resetting request store for partition '%X'", id)
	rs.reset()
}

// newRequestStore creates a new empty requestStore.
func newRequestStore() *requestStore {
	s := &requestStore{
		requests:   make(map[string]*certification.BlockCertificationRequest),
		hashCounts: make(map[string]uint)}
	return s
}

// add stores a new input record received from the node.
func (rs *requestStore) add(req *certification.BlockCertificationRequest) error {
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
	hasher := gocrypto.SHA256.New()
	req.InputRecord.AddToHasher(hasher)
	// remove to replace
	// todo: seems useless at the moment since if f is true, then we do not get here as error is returned before?
	// todo: spec says that the request can be replaced with newer, but what makes a request newer, round number?
	if _, f := rs.requests[req.NodeIdentifier]; f {
		rs.remove(req.NodeIdentifier)
	}
	hashString := string(hasher.Sum(nil))
	rs.requests[req.NodeIdentifier] = req
	count := rs.hashCounts[hashString]
	rs.hashCounts[hashString] = count + 1
	logger.Debug("Added new IR hash: %X, block hash: %X, total hash count: %v", req.InputRecord.Hash, req.InputRecord.BlockHash, rs.hashCounts[hashString])
	return nil
}

func (rs *requestStore) reset() {
	rs.requests = make(map[string]*certification.BlockCertificationRequest)
	rs.hashCounts = make(map[string]uint)
}

func (rs *requestStore) isConsensusReceived(nrOfNodes int) (*certificates.InputRecord, bool) {
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
		logger.Debug("isConsensusReceived: no hashes received yet, consensus possible in the future")
		return nil, true
	}

	needed := float64(nrOfNodes) / float64(2)
	logger.Debug("isConsensusReceived: count: %v, needed count: %v, hash:%X", c, needed, h)
	if float64(c) > needed {
		// consensus received
		logger.Debug("isConsensusReceived: yes")
		for _, req := range rs.requests {
			hasher := gocrypto.SHA256.New()
			req.InputRecord.AddToHasher(hasher)
			if bytes.Equal(h, hasher.Sum(nil)) {
				logger.Debug("isConsensusReceived: returning IR (hash: %X, block hash: %X)", req.InputRecord.Hash, req.InputRecord.BlockHash)
				return req.InputRecord, true
			}
		}
	} else if float64(uint(nrOfNodes)-uint(len(rs.requests))+c) <= needed {
		logger.Debug("isConsensusReceived: consensus not possible")
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	logger.Debug("isConsensusReceived: consensus possible in the future")
	return nil, true
}

func (rs *requestStore) remove(nodeId string) {
	oldReq, f := rs.requests[nodeId]
	if !f {
		return
	}
	hashString := string(oldReq.InputRecord.Hash)
	rs.hashCounts[hashString] = rs.hashCounts[hashString] - 1
	logger.Debug("Removing old IR hash: %X, block hash: %X, new hash count: %v", oldReq.InputRecord.Hash, oldReq.InputRecord.BlockHash, rs.hashCounts[hashString])
	delete(rs.requests, nodeId)
}
