package partition

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

const UnknownLeader = ""

var ErrPeerIsNilIndex = errors.New("peer is nil")

type (
	LeaderSelector interface {
		UpdateLeader(seal *certificates.UnicitySeal)
		IsCurrentNodeLeader() bool
		SelfID() peer.ID
	}

	// DefaultLeaderSelector is used to select a leader from the validator pool.
	DefaultLeaderSelector struct {
		mutex    sync.Mutex
		leader   peer.ID // current leader ID
		self     *network.Peer
		eventBus *eventbus.EventBus
	}
)

func NewDefaultLeaderSelector(self *network.Peer, eb *eventbus.EventBus) (*DefaultLeaderSelector, error) {
	if self == nil {
		return nil, ErrPeerIsNilIndex
	}
	if eb == nil {
		return nil, ErrEventBusIsNil
	}
	return &DefaultLeaderSelector{self: self, leader: UnknownLeader, eventBus: eb}, nil
}

func (l *DefaultLeaderSelector) SelfID() peer.ID {
	return l.self.ID()
}

func (l *DefaultLeaderSelector) GetLeader() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

// IsCurrentNodeLeader returns true it current node is the leader and must propose the next block.
func (l *DefaultLeaderSelector) IsCurrentNodeLeader() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	logger.Info("leader: %v, current node %v", l.leader, l.self.ID())
	return l.leader == l.self.ID()
}

// UpdateLeader updates the next block proposer. If input is nil then leader is set to UnknownLeader.
func (l *DefaultLeaderSelector) UpdateLeader(seal *certificates.UnicitySeal) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if seal == nil {
		l.setLeader(UnknownLeader)

		logger.Info("Leader set to unknown")
		return
	}
	peerCount := uint64(len(l.self.Configuration().PersistentPeers))
	index := seal.RootChainRoundNumber % peerCount
	if index > peerCount {
		l.setLeader(UnknownLeader)
		logger.Warning("Invalid leader index. Leader set to 'unknown'")
		return
	}
	leader, err := l.self.Configuration().PersistentPeers[index].GetID()
	if err != nil {
		l.setLeader(UnknownLeader)
		logger.Warning("Invalid leader index. Leader set to 'unknown'")
		return
	}
	l.setLeader(leader)
	logger.Info("New leader is %v", leader)
	return
}

func (l *DefaultLeaderSelector) setLeader(leader peer.ID) {
	l.leader = leader
	err := l.eventBus.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: l.leader})
	if err != nil {
		logger.Warning("Failed to submit leader change event: %v", err)
	}
}
