package partition

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

const UnknownLeader = ""

type Leader struct {
	mutex  sync.Mutex
	leader peer.ID
}

func (l *Leader) Set(leader peer.ID) {
	l.mutex.Lock()
	l.leader = leader
	l.mutex.Unlock()
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
