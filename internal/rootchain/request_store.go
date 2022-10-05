package rootchain

import (
	"bytes"

	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

type CertificationRequestStore map[p.SystemIdentifier]*RequestStore

// RequestStore keeps track of received consensus requests.
type RequestStore struct {
	id         p.SystemIdentifier
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// Get returns an existing store for systemid or registers and returns a new one if none existed
func (c CertificationRequestStore) Get(id p.SystemIdentifier) *RequestStore {
	store, f := c[id]
	if !f {
		store = NewRequestStore(id)
		c[id] = store
	}
	return store
}

// Reset removed all incoming requests from all stores
func (c CertificationRequestStore) Reset() {
	for _, store := range c {
		store.Reset()
	}
}

// NewRequestStore creates a new empty requestStore.
func NewRequestStore(systemIdentifier p.SystemIdentifier) *RequestStore {
	s := &RequestStore{id: systemIdentifier}
	s.Reset()
	return s
}

// Add stores a new input record received from the node.
func (rs *RequestStore) Add(nodeId string, req *certification.BlockCertificationRequest) error {
	prevReq, f := rs.requests[nodeId]
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
	// remove to replace, but there should not be any since if f is true, then we do not get here?
	if _, f := rs.requests[nodeId]; f {
		rs.Remove(nodeId)
	}
	hashString := string(req.InputRecord.Hash)
	rs.requests[nodeId] = req
	count := rs.hashCounts[hashString]
	rs.hashCounts[hashString] = count + 1
	logger.Debug("Added new IR hash: %X, block hash: %X, total hash count: %v", req.InputRecord.Hash, req.InputRecord.BlockHash, rs.hashCounts[hashString])
	return nil
}

func (rs *RequestStore) Reset() {
	logger.Debug("Resetting request store for partition '%X'", rs.id)
	rs.requests = make(map[string]*certification.BlockCertificationRequest)
	rs.hashCounts = make(map[string]uint)
}

func (rs *RequestStore) IsConsensusReceived(nrOfNodes int) (*certificates.InputRecord, bool) {
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
			if bytes.Equal(h, req.InputRecord.Hash) {
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

func (rs *RequestStore) Remove(nodeId string) {
	oldReq, f := rs.requests[nodeId]
	if !f {
		return
	}
	hashString := string(oldReq.InputRecord.Hash)
	rs.hashCounts[hashString] = rs.hashCounts[hashString] - 1
	logger.Debug("Removing old IR hash: %X, block hash: %X, new hash count: %v", oldReq.InputRecord.Hash, oldReq.InputRecord.BlockHash, rs.hashCounts[hashString])
	delete(rs.requests, nodeId)
}
