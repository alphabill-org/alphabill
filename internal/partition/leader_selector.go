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
		LeaderFromUnicitySeal(seal *certificates.UnicitySeal) peer.ID
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

// IsCurrentNodeLeader returns true it current node is the leader and must propose the next block.
func (l *DefaultLeaderSelector) IsCurrentNodeLeader() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	logger.Info("current leader: '%v', current node '%v'", l.leader, l.self.ID())
	return l.leader == l.self.ID()
}

// UpdateLeader updates the next block proposer. If input is nil then leader is set to UnknownLeader.
func (l *DefaultLeaderSelector) UpdateLeader(seal *certificates.UnicitySeal) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	newLeader := l.LeaderFromUnicitySeal(seal)
	l.setLeader(newLeader)
	logger.Info("Leader set to %v", newLeader)
}

func (l *DefaultLeaderSelector) LeaderFromUnicitySeal(seal *certificates.UnicitySeal) peer.ID {
	// We don't need the lock because we don't change the state of the struct.
	if seal == nil {
		return UnknownLeader
	}
	peerCount := uint64(len(l.self.Configuration().PersistentPeers))
	index := seal.RootChainRoundNumber % peerCount
	if index > peerCount {
		logger.Warning("Invalid leader index.")
		return UnknownLeader
	}
	leader, err := l.self.Configuration().PersistentPeers[index].GetID()
	if err != nil {
		logger.Warning("Invalid leader index: %v", err)
		return UnknownLeader
	}
	return leader
}

func (l *DefaultLeaderSelector) setLeader(leader peer.ID) {
	l.leader = leader
	err := l.eventBus.Submit(eventbus.TopicLeaders, eventbus.NewLeaderEvent{NewLeader: l.leader})
	if err != nil {
		logger.Warning("Failed to submit leader change event: %v", err)
	}
}
