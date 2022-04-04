package rootchain

import (
	"bytes"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
)

// requestStore keeps track of received consensus requests.
type requestStore struct {
	requests   map[string]*p1.RequestEvent // all received requests. key is node identifier
	hashCounts map[string]uint             // counts of requests with matching state. key is IR hash string.
}

// newRequestStore creates a new empty requestStore.
func newRequestStore() *requestStore {
	s := &requestStore{}
	s.reset()
	return s
}

// Add stores a new input record received from the node.
func (pr *requestStore) add(nodeId string, e *p1.RequestEvent) {
	if _, f := pr.requests[nodeId]; f {
		pr.remove(nodeId)
	}
	hashString := string(e.Req.InputRecord.Hash)
	pr.requests[nodeId] = e
	if count, found := pr.hashCounts[hashString]; found {
		pr.hashCounts[hashString] = count + 1
	} else {
		pr.hashCounts[hashString] = 1
	}
}

func (pr *requestStore) reset() {
	pr.requests = make(map[string]*p1.RequestEvent)
	pr.hashCounts = make(map[string]uint)
}

func (pr *requestStore) isConsensusReceived(nrOfNodes int) (*certificates.InputRecord, bool) {
	var h []byte
	var c uint = 0
	for hash, count := range pr.hashCounts {
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
		for _, event := range pr.requests {
			if bytes.Equal(h, event.Req.InputRecord.Hash) {
				return event.Req.InputRecord, true
			}
		}
	} else if float64(uint(nrOfNodes)-uint(len(pr.requests))+c) <= needed {
		// consensus not possible
		return nil, false
	}
	// consensus possible in the future
	return nil, true
}

func (pr *requestStore) remove(nodeId string) {
	oldReq, f := pr.requests[nodeId]
	if !f {
		return
	}
	hashString := string(oldReq.Req.InputRecord.Hash)
	pr.hashCounts[hashString] = pr.hashCounts[hashString] - 1
	delete(pr.requests, nodeId)
}
