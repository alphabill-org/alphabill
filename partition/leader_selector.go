package partition

import (
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

const UnknownLeader = ""

type Leader struct {
	mutex  sync.Mutex
	leader peer.ID
}

func (l *Leader) Set(nodeID string) error {
	id, err := peer.Decode(nodeID)
	if err != nil {
		return fmt.Errorf("decoding node ID %q: %w", nodeID, err)
	}

	l.mutex.Lock()
	l.leader = id
	l.mutex.Unlock()
	return nil
}

func (l *Leader) Get() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

func (l *Leader) IsLeader(peerID peer.ID) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader == peerID
}
