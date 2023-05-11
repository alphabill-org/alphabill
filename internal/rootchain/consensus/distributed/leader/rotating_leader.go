package leader

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const UnknownLeader = ""

var ErrPeerIsNil = errors.New("peer is nil")

type RotatingLeader struct {
	self      *network.Peer
	nofRounds uint32
}

// NewRotatingLeader returns round-robin leader selection algorithm based on node identifiers.
// It is assumed that the order of node identifiers is the same (e.g. alphabetical) for all validators
// "contRounds" - for how many continuous rounds each peer is considered to be the leader.
func NewRotatingLeader(self *network.Peer, contRounds uint32) (*RotatingLeader, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if len(self.Configuration().PersistentPeers) < 1 {
		return nil, errors.New("empty root validator node id list")
	}
	if contRounds < 1 || contRounds > uint32(len(self.Configuration().PersistentPeers)) {
		return nil, errors.New("invalid nof rounds")
	}
	return &RotatingLeader{self: self, nofRounds: contRounds}, nil
}

func (r *RotatingLeader) GetLeaderForRound(round uint64) peer.ID {
	index := uint32(round/uint64(r.nofRounds)) % uint32(len(r.self.Configuration().PersistentPeers))
	leader, err := r.self.Configuration().PersistentPeers[index].GetID()
	if err != nil {
		return UnknownLeader
	}
	return leader
}

func (r *RotatingLeader) GetRootNodes() []*network.PeerInfo {
	return r.self.Configuration().PersistentPeers
}
