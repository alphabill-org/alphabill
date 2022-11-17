package leader

import (
	"errors"
	"sort"

	"github.com/libp2p/go-libp2p/core/peer"
)

type RotatingLeader struct {
	rootNodeIds []peer.ID
	nofRounds   uint32
}

// NewRotatingLeader returns round-robin leader selection algorithm based on node identifiers.
// It is assumed that the order of node identifiers is the same (e.g. alphabetical) for all validators
func NewRotatingLeader(rootNodes []peer.ID, contRounds uint32) (*RotatingLeader, error) {
	if len(rootNodes) < 1 {
		return nil, errors.New("empty root validator node id list")
	}
	if contRounds < 1 || contRounds > uint32(len(rootNodes)) {
		return nil, errors.New("invalid nof rounds")
	}
	// For a simple round-robin we need a deterministic order that is the same in
	// every root validator - so simply sort by alphabet
	sort.Slice(rootNodes, func(i, j int) bool {
		return rootNodes[i] < rootNodes[j]
	})
	return &RotatingLeader{rootNodeIds: rootNodes, nofRounds: contRounds}, nil
}

func (r *RotatingLeader) IsValidLeader(author peer.ID, round uint64) bool {
	if r.GetLeaderForRound(round) != author {
		return false
	}
	return true
}

func (r *RotatingLeader) GetLeaderForRound(round uint64) peer.ID {
	return r.rootNodeIds[uint32(round/uint64(r.nofRounds))%uint32(len(r.rootNodeIds))]
}

func (r *RotatingLeader) GetRootNodes() []peer.ID {
	return r.rootNodeIds
}
