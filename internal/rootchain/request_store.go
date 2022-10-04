package rootchain

import (
	"bytes"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

// requestStore keeps track of received consensus requests.
type requestStore struct {
	id         p.SystemIdentifier
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// newRequestStore creates a new empty requestStore.
func newRequestStore(systemIdentifier p.SystemIdentifier) *requestStore {
	s := &requestStore{id: systemIdentifier}
	s.reset()
	return s
}

// Add stores a new input record received from the node.
func (rs *requestStore) add(nodeId string, req *certification.BlockCertificationRequest) {
	if _, f := rs.requests[nodeId]; f {
		rs.remove(nodeId)
	}
	hashString := string(req.InputRecord.Hash)
	rs.requests[nodeId] = req
	count := rs.hashCounts[hashString]
	rs.hashCounts[hashString] = count + 1
	logger.Debug("Added new IR hash: %X, block hash: %X, total hash count: %v", req.InputRecord.Hash, req.InputRecord.BlockHash, rs.hashCounts[hashString])
}

func (rs *requestStore) reset() {
	logger.Debug("Resetting request store for partition '%X'", rs.id)
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
