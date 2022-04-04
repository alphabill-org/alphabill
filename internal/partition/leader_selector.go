package partition

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

const UnknownLeader = ""

var (
	ErrInvalidLeaderIndex = errors.New("invalid leader index")
	ErrPeerIsNilIndex     = errors.New("peer is nil")
)

// LeaderSelector is used to select a leader from the validator pool.
type LeaderSelector struct {
	mutex  sync.Mutex
	leader peer.ID // current leader ID
	self   *network.Peer
}

func NewLeaderSelector(self *network.Peer) (*LeaderSelector, error) {
	if self == nil {
		return nil, ErrPeerIsNilIndex
	}
	return &LeaderSelector{self: self, leader: UnknownLeader}, nil
}

func (l *LeaderSelector) SelfID() peer.ID {
	return l.self.ID()
}

func (l *LeaderSelector) GetLeader() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

// IsCurrentNodeLeader returns true it current node is the leader and must propose the next block.
func (l *LeaderSelector) IsCurrentNodeLeader() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	logger.Info("leader: %v, current node %v", l.leader, l.self.ID())
	return l.leader == l.self.ID()
}

// UpdateLeader updates the next block proposer. If input is nil then leader is set to UnknownLeader.
func (l *LeaderSelector) UpdateLeader(uc *UnicitySeal) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if uc == nil {
		l.leader = UnknownLeader
		logger.Info("Leader set to unknown")
		return nil
	}
	peerCount := uint64(len(l.self.Configuration().PersistentPeers))
	index := uc.RootChainBlockNumber % peerCount
	if index > peerCount {
		l.leader = UnknownLeader
		logger.Warning("Invalid leader index. Leader set to 'unknown'")
		return ErrInvalidLeaderIndex
	}
	leader, err := l.self.Configuration().PersistentPeers[index].GetID()
	if err != nil {
		return err
	}
	l.leader = leader
	logger.Info("New leader is %v", leader)
	return nil
}
