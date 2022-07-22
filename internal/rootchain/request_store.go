package rootchain

import (
	"bytes"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

// requestStore keeps track of received consensus requests.
type requestStore struct {
	requests   map[string]*certification.BlockCertificationRequest // all received requests. key is node identifier
	hashCounts map[string]uint                                     // counts of requests with matching State. key is IR hash string.
}

// newRequestStore creates a new empty requestStore.
func newRequestStore() *requestStore {
	s := &requestStore{}
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
	logger.Debug("Added new IR hash: %X, block hash: %X", req.InputRecord.Hash, req.InputRecord.BlockHash)
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
		return nil, true
	}

	needed := float64(nrOfNodes) / float64(2)
	if float64(c) > needed {
		// consensus received
		for _, req := range rs.requests {
			if bytes.Equal(h, req.InputRecord.Hash) {
				return req.InputRecord, true
			}
		}
	} else if float64(uint(nrOfNodes)-uint(len(rs.requests))+c) <= needed {
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	return nil, true
}

func (rs *requestStore) remove(nodeId string) {
	oldReq, f := rs.requests[nodeId]
	if !f {
		return
	}
	hashString := string(oldReq.InputRecord.Hash)
	logger.Debug("Removing old IR hash: %X, block hash: %X", oldReq.InputRecord.Hash, oldReq.InputRecord.BlockHash)
	rs.hashCounts[hashString] = rs.hashCounts[hashString] - 1
	delete(rs.requests, nodeId)
}
